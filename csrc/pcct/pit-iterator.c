#include "pit-iterator.h"
#include "pit.h"

bool
PitDnUpIt_Extend_(PitDnUpIt_* it, Pit* pit, int maxInExt, size_t offsetInExt) {
  NDNDPDK_ASSERT(it->i == it->max);
  NDNDPDK_ASSERT(*it->nextPtr == NULL);

  // allocate PitEntryExt
  PitEntryExt* ext;
  int res = rte_mempool_get(Pcct_FromPit(pit)->mp, (void**)&ext);
  if (unlikely(res != 0)) {
    return false;
  }

  // clear PitEntryExt
  ext->dns[0].face = 0;
  ext->ups[0].face = 0;
  ext->next = NULL;

  // chain after PitEntry or existing PitEntryExt
  *it->nextPtr = ext;

  // update iterator
  it->i = 0;
  it->max = maxInExt;
  it->array = RTE_PTR_ADD(ext, offsetInExt);
  it->nextPtr = &ext->next;
  return true;
}
