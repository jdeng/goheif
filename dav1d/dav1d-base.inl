#include "src/decode.c"

#include "src/cdf.c"
#include "src/cpu.c"
#include "src/ctx.c"
#include "src/data.c"
#include "src/dequant_tables.c"
#include "src/getbits.c"
#include "src/intra_edge.c"
#include "src/itx_1d.c"
#include "src/lf_mask.c"
#include "src/lib.c"
#include "src/log.c"
#include "src/mem.c"
#include "src/msac.c"
#include "src/obu.c"
#include "src/pal.c"
#include "src/picture.c"
#include "src/qm.c"
#include "src/ref.c"
#include "src/refmvs.c"

#define init_internal init_internal_scan
#include "src/scan.c"
#undef init_internal

#include "src/tables.c"
#include "src/thread_task.c"
#include "src/warpmv.c"

#define transpose wedge_transpose
#include "src/wedge.c"
#undef transpose

