#ifndef NDN_DPDK_NDN_NAME_H
#define NDN_DPDK_NDN_NAME_H

/// \file

#include "namehash.h"
#include "tlv-element.h"

/** \brief Maximum supported name length (TLV-LENGTH of Name element).
 */
#define NAME_MAX_LENGTH 2048

/** \brief Name in linear buffer.
 */
typedef struct LName
{
  const uint8_t* value;
  uint16_t length;
} LName;

uint64_t __LName_ComputeHash(uint16_t length, const uint8_t* value);

/** \brief Compute hash for a name.
 */
static inline uint64_t
LName_ComputeHash(LName n)
{
  return __LName_ComputeHash(n.length, n.value);
}

/** \brief Indicate the result of name comparison.
 */
typedef enum NameCompareResult {
  NAMECMP_LT = -2,      ///< \p lhs is less than, but not a prefix of \p rhs
  NAMECMP_LPREFIX = -1, ///< \p lhs is a prefix of \p rhs
  NAMECMP_EQUAL = 0,    ///< \p lhs and \p rhs are equal
  NAMECMP_RPREFIX = 1,  ///< \p rhs is a prefix of \p lhs
  NAMECMP_GT = 2        ///< \p rhs is less than, but not a prefix of \p lhs
} NameCompareResult;

/** \brief Compare two names for <, ==, >, and prefix relations.
 */
NameCompareResult LName_Compare(LName lhs, LName rhs);

/** \brief Number of name components whose information are cached in Name struct
 *         for efficient processing.
 */
#define NAME_N_CACHED_COMPS 18

/** \brief Parsed Name element.
 */
typedef struct PName
{
  uint16_t nOctets;   ///< TLV-LENGTH of Name element
  uint16_t nComps;    ///< number of components
  bool hasDigestComp; ///< ends with digest component?

  bool hasHashes;                     ///< (pvt) are hash[i] computed?
  uint16_t comp[NAME_N_CACHED_COMPS]; ///< (pvt) end offset of i-th component
  uint64_t hash[NAME_N_CACHED_COMPS]; ///< (pvt) hash of i+1-component prefix
} PName;
static_assert(sizeof(PName) <= 3 * RTE_CACHE_LINE_SIZE, "");

/** \brief Parse a name from memory buffer.
 *  \param length TLV-LENGTH of Name element
 *  \param value TLV-VALUE of Name element
 *  \retval NdnError_OK success
 *  \retval NdnError_NameTooLong TLV-LENGTH exceeds \p NAME_MAX_LENGTH
 *  \retval NdnError_BadNameComponentType component type not in 1-32767 range
 *  \retval NdnError_BadDigestComponentLength ImplicitSha256DigestComponent is not 32 octets
 *  \retval NdnError_NameHasComponentAfterDigest ImplicitSha256DigestComponent is not at last
 */
NdnError PName_Parse(PName* n, uint32_t length, const uint8_t* value);

/** \brief Parse a name from TlvElement.
 *  \param ele TLV Name element, TLV-TYPE must be TT_Name
 *  \retval NdnError_Fragmented TLV-VALUE is not in consecutive memory
 *  \return return value of \p PName_Parse
 */
NdnError PName_FromElement(PName* n, const TlvElement* ele);

uint16_t __PName_SeekCompEnd(const PName* n, const uint8_t* input, uint16_t i);

/** \brief Get past-end offset of i-th component.
 *  \param input a buffer containing TLV-VALUE of Name element
 *  \param i component index, must be less than n->nComps
 */
static uint16_t
PName_GetCompEnd(const PName* n, const uint8_t* input, uint16_t i)
{
  assert(i < n->nComps);
  if (likely(i < NAME_N_CACHED_COMPS)) {
    return n->comp[i];
  }
  if (i == n->nComps - 1) {
    return n->nOctets;
  }
  return __PName_SeekCompEnd(n, input, i);
}

/** \brief Get start offset of i-th component.
 *  \param input a buffer containing TLV-VALUE of Name element
 *  \param i component index, must be less than n->nComps
 */
static uint16_t
PName_GetCompStart(const PName* n, const uint8_t* input, uint16_t i)
{
  assert(i < n->nComps);
  if (i == 0) {
    return 0;
  }
  return PName_GetCompEnd(n, input, i - 1);
}

void __PName_HashToCache(PName* n, const uint8_t* input);

/** \brief Compute hash for a prefix with i components.
 *  \param input a buffer containing TLV-VALUE of Name element
 *  \param i prefix length, must be no greater than n->nComps
 */
static uint64_t
PName_ComputePrefixHash(const PName* n, const uint8_t* input, uint16_t i)
{
  if (i == 0) {
    return NAMEHASH_EMPTYHASH;
  }

  assert(i <= n->nComps);
  if (unlikely(i > NAME_N_CACHED_COMPS)) {
    return __LName_ComputeHash(PName_GetCompEnd(n, input, i - 1), input);
  }

  if (!n->hasHashes) {
    __PName_HashToCache((PName*)n, input);
  }
  return n->hash[i - 1];
}

/** \brief Compute hash for whole name.
 *  \param input a buffer containing TLV-VALUE of Name element
 */
static uint64_t
PName_ComputeHash(const PName* n, const uint8_t* input)
{
  return PName_ComputePrefixHash(n, input, n->nComps);
}

#endif // NDN_DPDK_NDN_NAME_H