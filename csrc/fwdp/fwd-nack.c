#include "fwd.h"
#include "strategy.h"
#include "token.h"

#include "../core/logger.h"
#include "../pcct/pit-iterator.h"

INIT_ZF_LOG(FwFwd);

__attribute__((nonnull)) static void
FwFwd_TxNacks(FwFwd* fwd, PitEntry* pitEntry, TscTime now, NackReason reason, uint8_t nackHopLimit)
{
  PitDnIt it;
  for (PitDnIt_Init(&it, pitEntry); PitDnIt_Valid(&it); PitDnIt_Next(&it)) {
    PitDn* dn = it.dn;
    if (dn->face == 0) {
      break;
    }
    if (dn->expiry < now) {
      continue;
    }

    if (unlikely(Face_IsDown(dn->face))) {
      ZF_LOGD("^ no-nack-to=%" PRI_FaceID " drop=face-down", dn->face);
      continue;
    }

    InterestGuiders guiders = {
      .nonce = dn->nonce,
      .hopLimit = nackHopLimit,
    };
    Packet* output =
      Interest_ModifyGuiders(pitEntry->npkt, guiders, &fwd->mp, Face_PacketTxAlign(dn->face));
    if (unlikely(output == NULL)) {
      ZF_LOGD("^ no-nack-to=%" PRI_FaceID " drop=alloc-error", dn->face);
      break;
    }

    output = Nack_FromInterest(output, reason);
    Packet_GetLpL3Hdr(output)->pitToken = dn->token;
    ZF_LOGD("^ nack-to=%" PRI_FaceID " reason=%s npkt=%p nonce=%08" PRIx32 " dn-token=%016" PRIx64,
            dn->face, NackReason_ToString(reason), output, dn->nonce, dn->token);
    Face_Tx(dn->face, output);
  }
}

void
SgReturnNacks(SgCtx* ctx0, SgNackReason reason)
{
  FwFwdCtx* ctx = (FwFwdCtx*)ctx0;
  NDNDPDK_ASSERT(ctx->eventKind == SGEVT_INTEREST);

  FwFwd_TxNacks(ctx->fwd, ctx->pitEntry, rte_get_tsc_cycles(), (NackReason)reason, 1);
}

__attribute__((nonnull)) static bool
FwFwd_RxNackDuplicate(FwFwd* fwd, FwFwdCtx* ctx)
{
  TscTime now = rte_get_tsc_cycles();
  PitUp* up = ctx->pitUp;
  PitUp_AddRejectedNonce(up, up->nonce);

  InterestGuiders guiders = {
    .nonce = up->nonce,
    .lifetime = PitEntry_GetTxInterestLifetime(ctx->pitEntry, now),
    .hopLimit = PitEntry_GetTxInterestHopLimit(ctx->pitEntry),
  };
  bool hasAltNonce = PitUp_ChooseNonce(up, ctx->pitEntry, now, &guiders.nonce);
  if (!hasAltNonce) {
    return false;
  }

  Packet* outNpkt =
    Interest_ModifyGuiders(ctx->pitEntry->npkt, guiders, &fwd->mp, Face_PacketTxAlign(up->face));
  if (unlikely(outNpkt == NULL)) {
    ZF_LOGD("^ no-interest-to=%" PRI_FaceID " drop=alloc-error", up->face);
    return true;
  }

  uint64_t token = FwToken_New(fwd->id, PitEntry_GetToken(ctx->pitEntry));
  Packet_GetLpL3Hdr(outNpkt)->pitToken = token;
  Packet_ToMbuf(outNpkt)->timestamp = ctx->pkt->timestamp; // for latency stats

  ZF_LOGD("^ interest-to=%" PRI_FaceID " npkt=%p " PRI_InterestGuiders " up-token=%016" PRIx64,
          up->face, outNpkt, InterestGuiders_Fmt(guiders), token);
  Face_Tx(up->face, outNpkt);
  if (ctx->fibEntryDyn != NULL) {
    ++ctx->fibEntryDyn->nTxInterests;
  }

  PitUp_RecordTx(up, ctx->pitEntry, now, guiders.nonce, &fwd->suppressCfg);
  return true;
}

__attribute__((nonnull)) static void
FwFwd_ProcessNack(FwFwd* fwd, FwFwdCtx* ctx)
{
  PNack* nack = Packet_GetNackHdr(ctx->npkt);
  NackReason reason = nack->lpl3.nackReason;
  uint8_t nackHopLimit = nack->interest.hopLimit;

  ZF_LOGD("nack-from=%" PRI_FaceID " npkt=%p up-token=%016" PRIx64 " reason=%" PRIu8, ctx->rxFace,
          ctx->npkt, ctx->rxToken, reason);

  // find PIT entry
  ctx->pitEntry = Pit_FindByNack(fwd->pit, ctx->npkt);
  if (unlikely(ctx->pitEntry == NULL)) {
    ZF_LOGD("^ drop=no-PIT-entry");
    return;
  }

  // verify nonce in Nack matches nonce in PitUp
  // count remaining pending upstreams and find least severe Nack reason
  int nPending = 0;
  NackReason leastSevere = reason;
  PitUpIt it;
  for (PitUpIt_Init(&it, ctx->pitEntry); PitUpIt_Valid(&it); PitUpIt_Next(&it)) {
    if (it.up->face == 0) {
      continue;
    }
    if (it.up->face == ctx->rxFace) {
      if (unlikely(it.up->nonce != nack->interest.nonce)) {
        ZF_LOGD("^ drop=wrong-nonce pit-nonce=%" PRIx32 " up-nonce=%" PRIx32, it.up->nonce,
                nack->interest.nonce);
        break;
      }
      ctx->pitUp = it.up;
      continue;
    }

    if (it.up->nack == NackNone) {
      ++nPending;
    } else {
      leastSevere = NackReason_GetMin(leastSevere, it.up->nack);
    }
  }
  if (unlikely(ctx->pitUp == NULL)) {
    ++fwd->nNackMismatch;
    return;
  }

  // record NackReason in PitUp
  ctx->pitUp->nack = reason;

  // find FIB entry; FIB entry is optional for Nack processing
  rcu_read_lock();
  FwFwdCtx_SetFibEntry(ctx, PitEntry_FindFibEntry(ctx->pitEntry, fwd->fib));
  if (likely(ctx->fibEntry != NULL)) {
    ++ctx->fibEntryDyn->nRxNacks;
  }

  // Duplicate: record rejected nonce, resend with an alternate nonce if possible
  if (reason == NackDuplicate && FwFwd_RxNackDuplicate(fwd, ctx)) {
    FwFwd_NULLize(ctx->fibEntry); // fibEntry is inaccessible upon RCU unlock
    rcu_read_unlock();
    return;
  }

  // invoke strategy if FIB entry exists
  if (likely(ctx->fibEntry != NULL)) {
    // TODO set ctx->nhFlt to prevent forwarding to downstream
    uint64_t res = SgInvoke(ctx->fibEntry->strategy, ctx);
    ZF_LOGD("^ fib-entry-depth=%" PRIu8 " sg-id=%d sg-res=%" PRIu64, ctx->fibEntry->nComps,
            ctx->fibEntry->strategy->id, res);
  }
  FwFwd_NULLize(ctx->fibEntry); // fibEntry is inaccessible upon RCU unlock
  rcu_read_unlock();

  // if there are more pending upstream or strategy retries, wait for them
  if (nPending + ctx->nForwarded > 0) {
    ZF_LOGD("^ up-pendings=%d sg-forwarded=%d", nPending, ctx->nForwarded);
    return;
  }

  // return Nacks to downstream and erase PIT entry
  FwFwd_TxNacks(fwd, ctx->pitEntry, ctx->rxTime, leastSevere, nackHopLimit);
  Pit_Erase(fwd->pit, ctx->pitEntry);
  FwFwd_NULLize(ctx->pitEntry);
}

void
FwFwd_RxNack(FwFwd* fwd, FwFwdCtx* ctx)
{
  FwFwd_ProcessNack(fwd, ctx);
  rte_pktmbuf_free(ctx->pkt);
  FwFwd_NULLize(ctx->pkt);
}
