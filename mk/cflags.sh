export CC=${CC:-gcc-7}
export CGO_CFLAGS_ALLOW='.*'
CFLAGS='-Werror -Wno-error=deprecated-declarations -m64 -pthread -O3 -ffast-math -g -march=native -I/usr/local/include/dpdk -I/usr/include/dpdk'
LIBS='-L/usr/local/lib -lurcu-qsbr -lurcu-cds -lubpf -lspdk -lspdk_env_dpdk -ldpdk -lnuma -lm'

if ! [[ $MK_CGOFLAGS ]]; then
  CFLAGS='-Wall '$CFLAGS
fi

if [[ -n $RELEASE ]]; then
  CFLAGS=$CFLAGS' -DNDEBUG -DZF_LOG_DEF_LEVEL=ZF_LOG_INFO'
fi

if [[ $CC =~ clang ]]; then
  CFLAGS=$CFLAGS' -Wno-error=address-of-packed-member'
fi
