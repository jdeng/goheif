/*
 * H.265 video codec.
 * Copyright (c) 2013-2014 struktur AG, Dirk Farin <farin@struktur.de>
 *
 * This file is part of libde265.
 *
 * libde265 is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as
 * published by the Free Software Foundation, either version 3 of
 * the License, or (at your option) any later version.
 *
 * libde265 is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with libde265.  If not, see <http://www.gnu.org/licenses/>.
 */

#ifndef DE265_SSE_H
#define DE265_SSE_H

#include "acceleration.h"

void init_acceleration_functions_sse(struct acceleration_functions* accel);

// Overrides selected transform kernels with AVX2 versions, but only if the
// running CPU actually supports AVX2 (checked at runtime). Safe to call on any
// CPU; a no-op when AVX2 is unavailable.
void init_acceleration_functions_avx2(struct acceleration_functions* accel);

// Overrides selected transform kernels with AVX-512 versions, runtime-checked.
// Safe to call on any CPU; a no-op when AVX-512 is unavailable.
void init_acceleration_functions_avx512(struct acceleration_functions* accel);

#endif
