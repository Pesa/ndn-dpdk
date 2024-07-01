#include "rxtable.h"
#include "face.h"

__attribute__((nonnull)) static inline bool
EthRxTable_Accept(EthRxTable* rxt, struct rte_mbuf* m) {
  // RCU lock is inherited from RxLoop_Run
  EthFacePriv* priv;
  struct cds_hlist_node* pos;
  cds_hlist_for_each_entry_rcu (priv, pos, &rxt->head, rxtNode) {
    if (EthRxMatch_Match(&priv->rxMatch, m)) {
      m->port = priv->faceID;
      rte_pktmbuf_adj(m, priv->rxMatch.len);
      return true;
    }
  }
  return false;
}

void
EthRxTable_RxBurst(RxGroup* rxg, RxGroupBurstCtx* ctx) {
  EthRxTable* rxt = container_of(rxg, EthRxTable, base);
  ctx->nRx = rte_eth_rx_burst(rxt->port, rxt->queue, ctx->pkts, RTE_DIM(ctx->pkts));
  uint64_t now = rte_get_tsc_cycles();

  PdumpEthPortUnmatchedCtx unmatch;
  // RCU lock is inherited from RxLoop_Run
  PdumpEthPortUnmatchedCtx_Init(&unmatch, rxt->port);

  struct rte_mbuf* bounceBufs[MaxBurstSize];
  uint16_t nBounceBufs = 0;
  for (uint16_t i = 0; i < ctx->nRx; ++i) {
    struct rte_mbuf* m = ctx->pkts[i];
    Mbuf_SetTimestamp(m, now);
    if (unlikely(!EthRxTable_Accept(rxt, m))) {
      RxGroupBurstCtx_Drop(ctx, i);
      if (PdumpEthPortUnmatchedCtx_Append(&unmatch, m)) {
        ctx->pkts[i] = NULL;
      } else if (rxt->copyTo != NULL) {
        // free bounce bufs locally instead of via RxLoop, because rte_pktmbuf_free_bulk is most
        // efficient when consecutive mbufs are from the same mempool such as the main mempool
        bounceBufs[nBounceBufs++] = m;
        ctx->pkts[i] = NULL;
      }
      continue;
    }

    if (rxt->copyTo == NULL) {
      continue;
    }

    struct rte_mbuf* copy = rte_pktmbuf_copy(m, rxt->copyTo, 0, UINT32_MAX);
    ctx->pkts[i] = copy;
    if (unlikely(copy == NULL)) {
      RxGroupBurstCtx_Drop(ctx, i);
    }
    bounceBufs[nBounceBufs++] = m;
  }

  PdumpEthPortUnmatchedCtx_Process(&unmatch);
  if (unlikely(nBounceBufs > 0)) {
    rte_pktmbuf_free_bulk(bounceBufs, nBounceBufs);
  }
}

STATIC_ASSERT_FUNC_TYPE(RxGroup_RxBurstFunc, EthRxTable_RxBurst);
