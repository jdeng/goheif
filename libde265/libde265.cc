#include <stdint.h>
#include "libde265-all.inl"

// copied from https://github.com/strukturag/libheif/blob/master/libheif/heif_decoder_libde265.cc with modification
extern "C" int push_data(de265_decoder_context *ctx, const void *data, size_t size) {
  const uint8_t* cdata = (const uint8_t*)data;

  size_t ptr=0;
  while (ptr < size) {
    if (ptr+4 > size) {
      return -1;
    }

    uint32_t nal_size = (cdata[ptr]<<24) | (cdata[ptr+1]<<16) | (cdata[ptr+2]<<8) | (cdata[ptr+3]);
    ptr+=4;

    if (ptr+nal_size > size) {
      printf("size too big: %d,%d,%d\n", ptr, nal_size, size);
      return -1;
    }

    de265_push_NAL(ctx, cdata+ptr, nal_size, 0, nullptr);
    ptr += nal_size;
  }

   return 0;
}

