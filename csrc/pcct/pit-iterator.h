#ifndef NDNDPDK_PCCT_PIT_DN_UP_IT_H
#define NDNDPDK_PCCT_PIT_DN_UP_IT_H

/** @file */

#include "pit-entry.h"

typedef struct PitDnUpIt_ {
  union {
    PitDn* dn; ///< current PitDn
    PitUp* up; ///< current PitUp
  };
  int index; ///< index of PitDn/PitUp

  int i;   ///< (pvt) index within this array
  int max; ///< (pvt) upper bound of this array
  union {
    void* array; // (pvt) start of array
    PitDn* dns;
    PitUp* ups;
  };

  PitEntryExt** nextPtr; ///< (pvt) next extension
} PitDnUpIt_;

__attribute__((nonnull)) static inline void
PitDnUpIt_Init_(PitDnUpIt_* it, PitEntry* entry, int maxInEntry, size_t offsetInEntry) {
  it->index = 0;
  it->i = 0;
  it->max = maxInEntry;
  it->array = RTE_PTR_ADD(entry, offsetInEntry);
  it->nextPtr = &entry->ext;
}

__attribute__((nonnull)) static inline void
PitDnUpIt_Next_(PitDnUpIt_* it, int maxInExt, size_t offsetInExt) {
  NDNDPDK_ASSERT(it->i < it->max);
  ++it->index;
  ++it->i;
  if (likely(it->i < it->max)) {
    return;
  }

  PitEntryExt* ext = *it->nextPtr;
  if (ext == NULL) {
    return;
  }
  it->i = 0;
  it->max = maxInExt;
  it->array = RTE_PTR_ADD(ext, offsetInExt);
  it->nextPtr = &ext->next;
}

__attribute__((nonnull)) bool
PitDnUpIt_Extend_(PitDnUpIt_* it, Pit* pit, int maxInExt, size_t offsetInExt);

/**
 * @brief Iterator of DN slots in PIT entry.
 *
 * @code
 * PitDnIt it;
 * for (PitDnIt_Init(&it, entry); PitDnIt_Valid(&it); PitDnIt_Next(&it)) {
 *   int index = it.index;
 *   PitDn* dn = it.dn;
 * }
 * @endcode
 */
typedef PitDnUpIt_ PitDnIt;

__attribute__((nonnull)) static inline void
PitDnIt_Init(PitDnIt* it, PitEntry* entry) {
  PitDnUpIt_Init_(it, entry, PitMaxDns, offsetof(PitEntry, dns));
  it->dn = &it->dns[it->i];
}

__attribute__((nonnull)) static inline bool
PitDnIt_Valid(PitDnIt* it) {
  return it->i < it->max;
}

__attribute__((nonnull)) static inline void
PitDnIt_Next(PitDnIt* it) {
  PitDnUpIt_Next_(it, PitMaxExtDns, offsetof(PitEntryExt, dns));
  it->dn = &it->dns[it->i];
}

/**
 * @brief Add an extension for more DN slots.
 * @retval true extension added, iterator points to next slot.
 * @retval false allocation failure.
 */
__attribute__((nonnull)) static inline bool
PitDnIt_Extend(PitDnIt* it, Pit* pit) {
  bool ok = PitDnUpIt_Extend_(it, pit, PitMaxExtDns, offsetof(PitEntryExt, dns));
  it->dn = &it->dns[it->i];
  return ok;
}

/**
 * @brief Iterator of UP slots in PIT entry.
 *
 * @code
 * PitUpIt it;
 * for (PitUpIt_Init(&it, entry); PitUpIt_Valid(&it); PitUpIt_Next(&it)) {
 *   int index = it.index;
 *   PitUp* up = it.up;
 * }
 * @endcode
 */
typedef PitDnUpIt_ PitUpIt;

__attribute__((nonnull)) static inline void
PitUpIt_Init(PitUpIt* it, PitEntry* entry) {
  PitDnUpIt_Init_(it, entry, PitMaxUps, offsetof(PitEntry, ups));
  it->up = &it->ups[it->i];
}

__attribute__((nonnull)) static inline bool
PitUpIt_Valid(PitUpIt* it) {
  return it->i < it->max;
}

__attribute__((nonnull)) static inline void
PitUpIt_Next(PitUpIt* it) {
  PitDnUpIt_Next_(it, PitMaxExtUps, offsetof(PitEntryExt, ups));
  it->up = &it->ups[it->i];
}

/**
 * @brief Add an extension for more UP slots.
 * @retval true extension added, iterator points to next slot.
 * @retval false allocation failure.
 */
__attribute__((nonnull)) static inline bool
PitUpIt_Extend(PitDnIt* it, Pit* pit) {
  bool ok = PitDnUpIt_Extend_(it, pit, PitMaxExtUps, offsetof(PitEntryExt, ups));
  it->up = &it->ups[it->i];
  return ok;
}

#endif // NDNDPDK_PCCT_PIT_DN_UP_IT_H
