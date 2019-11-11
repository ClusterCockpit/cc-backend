CC   = clang
GCC  = gcc
LINKER = $(CC)

OPENMP   = -fopenmp
CFLAGS   = -Ofast -std=c99 $(OPENMP)
LFLAGS   = $(OPENMP)
DEFINES  = -D_GNU_SOURCE -DJSMN_PARENT_LINKS
INCLUDES =
LIBS     =
