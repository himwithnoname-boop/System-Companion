#pragma once
#include <float.h>

/* Returns LVGL's rotation index, or -1 to retain the current orientation.
 * With the display facing us: port bottom = +X, left = -Y, top = -X,
 * right = +Y. LVGL's software 90-degree rotation is counterclockwise.
 * The upright panel basis is therefore 270 degrees, with Y reversed.
 */
static inline int orientation_target(float x,float y,float z) {
    float ax=x<0?-x:x,ay=y<0?-y:y,az=z<0?-z:z;
    float dominant=ax>ay?ax:ay,difference=ax>ay?ax-ay:ay-ax;
    /* Comparisons also reject NaN, infinity and ambiguous/flat readings. */
    if(!(ax<=FLT_MAX&&ay<=FLT_MAX&&az<=.8f&&dominant>=.65f&&difference>=.2f)) return -1;
    return ax>ay?(x>0?3:1):(y>0?2:0);
}
