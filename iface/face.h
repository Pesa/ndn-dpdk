#ifndef NDN_DPDK_IFACE_FACE_H
#define NDN_DPDK_IFACE_FACE_H

#include "rx-proc.h"
#include "tx-proc.h"

/// \file

/** \brief Numeric face identifier.
 */
typedef uint16_t FaceId;

typedef struct Face Face;
typedef struct FaceCounters FaceCounters;

/** \brief Receive a burst of L2 frames.
 *  \param[out] pkts L2 frames without Ethernet/etc header;
 *                   callback releases ownership of these frames
 */
typedef uint16_t (*FaceOps_RxBurst)(Face* face, struct rte_mbuf** pkts,
                                    uint16_t nPkts);

/** \brief Transmit a burst of L2 frames.
 *  \param pkts L2 frames with NDNLP header
 *  \return successfully queued packets; callback owns queued frames, but does
 *          not own or release the remaining frames
 */
typedef uint16_t (*FaceOps_TxBurst)(Face* face, struct rte_mbuf** pkts,
                                    uint16_t nPkts);

/** \brief Close a face.
 */
typedef bool (*FaceOps_Close)(Face* face);

typedef struct FaceOps
{
  // most frequent ops, rxBurst and txBurst, are placed directly in Face struct
  FaceOps_Close close;
} FaceOps;

/** \brief Generic network interface.
 */
typedef struct Face
{
  FaceOps_RxBurst rxBurstOp;
  FaceOps_TxBurst txBurstOp;
  const FaceOps* ops;

  RxProc rx;
  TxProc tx;

  FaceId id;
} Face;

// ---- functions invoked user of face system ----

static inline bool
Face_Close(Face* face)
{
  return (*face->ops->close)(face);
}

/** \brief Receive and decode a burst of packet.
 *  \param face the face
 *  \param[out] pkts array of network layer packets with PacketPriv
 *  \param nPkts size of \p pkts array
 *  \return number of retrieved packets
 */
uint16_t Face_RxBurst(Face* face, struct rte_mbuf** pkts, uint16_t nPkts);

/** \brief Send a burst of packet.
 *  \param face the face
 *  \param pkts array of network layer packets with PacketPriv;
 *              this function does not take ownership of these packets
 *  \param nPkts size of \p pkt array
 */
void Face_TxBurst(Face* face, struct rte_mbuf** pkts, uint16_t nPkts);

/** \brief Retrieve face counters.
 */
void Face_ReadCounters(Face* face, FaceCounters* cnt);

// ---- functions invoked by face implementation ----

/** \brief Initialize a face.
 *
 *  This should be called after initialization.
 */
void FaceImpl_Init(Face* face, uint16_t mtu, uint16_t headroom,
                   struct rte_mempool* indirectMp,
                   struct rte_mempool* headerMp);

/** \brief Update counters after a frame is transmitted.
 *
 *  This should be called after transmitting \p pkt .
 */
static inline void
FaceImpl_CountSent(Face* face, struct rte_mbuf* pkt)
{
  TxProc_CountSent(&face->tx, pkt);
}

#endif // NDN_DPDK_IFACE_FACE_H
