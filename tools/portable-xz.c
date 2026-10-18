/* Small Linux-only offline XZ decoder. No libc or external decoder is needed.
 * Codec implementation: public-domain LZMA SDK, see tools/vendor/lzma/NOTICE. */
#include <stddef.h>
#include <stdint.h>
#include "Xz.h"
#include "7zCrc.h"
#include "XzCrc64.h"

#if defined(__x86_64__)
static long call6(long n, long a, long b, long c, long d, long e, long f) {
    register long r10 __asm__("r10")=d, r8 __asm__("r8")=e, r9 __asm__("r9")=f;
    long result;
    __asm__ volatile("syscall" : "=a"(result) : "a"(n), "D"(a), "S"(b), "d"(c), "r"(r10), "r"(r8), "r"(r9) : "rcx", "r11", "memory");
    return result;
}
#define SYS_READ 0
#define SYS_WRITE 1
#define SYS_MMAP 9
#define SYS_MUNMAP 11
#define SYS_EXIT 60
#define SYS_OPENAT 257
#elif defined(__aarch64__)
static long call6(long n, long a, long b, long c, long d, long e, long f) {
    register long x8 __asm__("x8")=n, x0 __asm__("x0")=a, x1 __asm__("x1")=b,
        x2 __asm__("x2")=c, x3 __asm__("x3")=d, x4 __asm__("x4")=e, x5 __asm__("x5")=f;
    __asm__ volatile("svc 0" : "+r"(x0) : "r"(x8), "r"(x1), "r"(x2), "r"(x3), "r"(x4), "r"(x5) : "memory");
    return x0;
}
#define SYS_READ 63
#define SYS_WRITE 64
#define SYS_MMAP 222
#define SYS_MUNMAP 215
#define SYS_EXIT 93
#define SYS_OPENAT 56
#else
#error Unsupported Linux architecture
#endif

void *memcpy(void *dest, const void *src, size_t n) { unsigned char *d=dest; const unsigned char *s=src; while(n--) *d++=*s++; return dest; }
void *memmove(void *dest, const void *src, size_t n) { unsigned char *d=dest; const unsigned char *s=src; if(d<s) return memcpy(dest,src,n); while(n) { --n; d[n]=s[n]; } return dest; }
void *memset(void *dest, int value, size_t n) { unsigned char *d=dest; while(n--) *d++=(unsigned char)value; return dest; }
int memcmp(const void *a, const void *b, size_t n) { const unsigned char *x=a,*y=b; while(n--) { if(*x!=*y) return *x-*y; ++x; ++y; } return 0; }
size_t strlen(const char *s) { size_t n=0; while(s[n]) ++n; return n; }
unsigned long getauxval(unsigned long type) { (void)type; return 0; }

void *malloc(size_t size) {
    if(size>128*1024*1024) return NULL;
    size_t length=(size+16+4095)&~(size_t)4095;
    long p=call6(SYS_MMAP,0,length,3,0x22,-1,0);
    if((unsigned long)p>(unsigned long)-4096) return NULL;
    *(size_t *)p=length;
    return (void *)(p+16);
}
void free(void *p) { if(p) { long base=(long)p-16; call6(SYS_MUNMAP,base,*(size_t *)base,0,0,0,0); } }
static void *allocate(ISzAllocPtr p, size_t size) { (void)p; return malloc(size); }
static void release(ISzAllocPtr p, void *data) { (void)p; free(data); }
static const ISzAlloc allocator={allocate,release};

__attribute__((noreturn)) static void stop(int code) {
    if(code) { const char *msg="System Companion: offline package decoding failed.\n"; call6(SYS_WRITE,2,(long)msg,strlen(msg),0,0,0); }
    call6(SYS_EXIT,code,0,0,0,0,0);
    __builtin_unreachable();
}
__attribute__((used,noreturn)) void entry(long *stack) {
    if(stack[0]!=2) stop(2);
    char **argv=(char **)(stack+1);
    long fd=call6(SYS_OPENAT,-100,(long)argv[1],0,0,0,0);
    if(fd<0) stop(2);
    const size_t inputLimit=12*1024*1024, outputLimit=64*1024*1024;
    Byte *input=malloc(inputLimit), *output=malloc(outputLimit);
    if(!input || !output) stop(2);
    size_t used=0;
    for(;;) {
        if(used==inputLimit) stop(2);
        long n=call6(SYS_READ,fd,(long)(input+used),inputLimit-used,0,0,0);
        if(n==-4) continue;
        if(n<0) stop(2);
        if(n==0) break;
        used+=(size_t)n;
    }
    CrcGenerateTable(); Crc64GenerateTable();
    CXzUnpacker unpacker; XzUnpacker_Construct(&unpacker,&allocator);
    SizeT source=used, target=outputLimit; ECoderStatus status;
    SRes result=XzUnpacker_CodeFull(&unpacker,output,&target,input,&source,CODER_FINISH_END,&status);
    if(result!=SZ_OK || source!=used || !XzUnpacker_IsStreamWasFinished(&unpacker)) stop(2);
    XzUnpacker_Free(&unpacker); free(input);
    size_t sent=0;
    while(sent<target) {
        long n=call6(SYS_WRITE,1,(long)(output+sent),target-sent,0,0,0);
        if(n==-4) continue;
        if(n<=0) stop(2);
        sent+=(size_t)n;
    }
    free(output); stop(0);
}

#if defined(__x86_64__)
__asm__(".global _start\n_start:\nmov %rsp,%rdi\nand $-16,%rsp\ncall entry\n");
#else
__asm__(".global _start\n_start:\nmov x0,sp\nbl entry\n");
#endif
