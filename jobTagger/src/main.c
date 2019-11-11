/*
 * =======================================================================================
 *
 *      Author:   Jan Eitzinger (je), jan.eitzinger@fau.de
 *      Copyright (c) 2019 RRZE, University Erlangen-Nuremberg
 *
 *      Permission is hereby granted, free of charge, to any person obtaining a copy
 *      of this software and associated documentation files (the "Software"), to deal
 *      in the Software without restriction, including without limitation the rights
 *      to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 *      copies of the Software, and to permit persons to whom the Software is
 *      furnished to do so, subject to the following conditions:
 *
 *      The above copyright notice and this permission notice shall be included in all
 *      copies or substantial portions of the Software.
 *
 *      THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 *      IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 *      FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 *      AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 *      LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 *      OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 *      SOFTWARE.
 *
 * =======================================================================================
 */

#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>
#include <limits.h>
#include <float.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/mman.h>
#include <fcntl.h>

#ifdef _OPENMP
#include <omp.h>
#endif

#include "jsmn.h"
#include "timing.h"
#include "allocate.h"
#include "affinity.h"

#define HLINE "----------------------------------------------------------------------------\n"

#ifndef MIN
#define MIN(x,y) ((x)<(y)?(x):(y))
#endif
#ifndef MAX
#define MAX(x,y) ((x)>(y)?(x):(y))
#endif
#ifndef ABS
#define ABS(a) ((a) >= 0 ? (a) : -(a))
#endif

char* json_fetch(char* filepath)
{
    int fd = open(filepath, O_RDONLY);
    if ( fd == -1) {
        perror("Cannot open output file\n"); exit(1);
    }
    int len = lseek(fd, 0, SEEK_END);
    void *data = mmap(0, len, PROT_READ, MAP_PRIVATE, fd, 0);

    return (char*) data;
}

jsmntok_t * json_tokenise(char *js)
{
    jsmn_parser parser;
    jsmn_init(&parser);

    unsigned int n = 4096;
    jsmntok_t *tokens = malloc(sizeof(jsmntok_t) * n);

    int ret = jsmn_parse(&parser, js, strlen(js), tokens, n);

    while (ret == JSMN_ERROR_NOMEM)
    {
        n = n * 2 + 1;
        tokens = realloc(tokens, sizeof(jsmntok_t) * n);
        ret = jsmn_parse(&parser, js, strlen(js), tokens, n);
    }

    if (ret == JSMN_ERROR_INVAL) {
        printf("jsmn_parse: invalid JSON string");
        exit(EXIT_SUCCESS);
    }
    if (ret == JSMN_ERROR_PART) {
        printf("jsmn_parse: truncated JSON string");
        exit(EXIT_SUCCESS);
    }

    return tokens;
}

int json_token_streq(char* js, jsmntok_t* t, char* s)
{
    return (strncmp(js + t->start, s, t->end - t->start) == 0
            && strlen(s) == (size_t) (t->end - t->start));
}

char* json_token_tostr(char* js, jsmntok_t* t)
{
    js[t->end] = '\0';
    return js + t->start;
}

void print_token(jsmntok_t* t)
{
    char* type;

    switch ( t->type ){
        case JSMN_STRING:
            type = "STRING";
            break;
        case JSMN_OBJECT:
            type = "OBJECT";
            break;
        case JSMN_ARRAY:
            type = "ARRAY";
            break;
        case JSMN_PRIMITIVE:
            type = "PRIMITIVE";
            break;
    }

    printf("%s: S%d E%d C%d\n", type, t->start, t->end, t->size);

}

int main (int argc, char** argv)
{
    char* filepath;

    if ( argc > 1 ) {
        filepath = argv[1];
    } else {
        printf("Usage: %s  <filepath>\n",argv[0]);
        exit(EXIT_SUCCESS);
    }

    char* js = json_fetch(filepath);
    jsmntok_t* tokens = json_tokenise(js);

    typedef enum {
        START,
        METRIC, METRIC_OBJECT,
        SERIES, NODE_ARRAY,
        NODE_OBJECT,
        DATA,
        SKIP,
        STOP
    } parse_state;

    parse_state state = START;
    size_t node_tokens = 0;
    size_t skip_tokens = 0;
    size_t metrics = 0;
    size_t nodes = 0;
    size_t elements = 0;

    for (size_t i = 0, j = 1; j > 0; i++, j--)
    {
        jsmntok_t* t = &tokens[i];

        if (t->type == JSMN_ARRAY || t->type == JSMN_OBJECT){
            j += t->size;
        }
        print_token(t);

        switch (state)
        {
           case START:
               if (t->type != JSMN_OBJECT){
                    printf("Invalid response: root element must be object.");
                    exit(EXIT_SUCCESS);
               }

                state = METRIC;
                break;

            case METRIC:
                if (t->type != JSMN_STRING){
                    printf("Invalid response: metric key must be a string.");
                    exit(EXIT_SUCCESS);
                }

                printf("METRIC\n");
                state = METRIC_OBJECT;
                object_tokens = t->size;
                break;

            case METRIC_OBJECT:
                printf("METRIC OBJECT %lu\n", object_tokens);
                object_tokens--;

                    if (t->type == JSMN_STRING && json_token_streq(js, t, "series")) {
                        state = SERIES;
                    } else {
                        state = SKIP;
                        if (t->type == JSMN_ARRAY || t->type == JSMN_OBJECT) {
                            skip_tokens = t->size;
                        }
                    }

                // Last object value
                if (object_tokens == 0) {
                    state = METRIC;
                }

                break;

            case SKIP:
                skip_tokens--;

                printf("SKIP\n");
                if (t->type == JSMN_ARRAY || t->type == JSMN_OBJECT) {
                    skip_tokens += t->size;
                }

                break;

            case SERIES:
                if (t->type != JSMN_ARRAY) {
                    printf("Unknown series value: expected array.");
                }

                printf("SERIES\n");
                nodes = t->size;
                state = NODE_ARRAY;

                if (nodes == 0) {
                    state = METRIC_OBJECT;
                }

                break;

            case NODE_ARRAY:
                nodes--;

                printf("NODE_ARRAY\n");
                node_tokens = t->size;
                state = NODE_OBJECT;

                // Last node object
                if (nodes == 0) {
                    state = STOP;
                }

                break;

            case NODE_OBJECT:
                node_tokens--;

                printf("NODE_OBJECT\n");
                // Keys are odd-numbered tokens within the object
                if (node_tokens % 2 == 1)
                {
                    if (t->type == JSMN_STRING && json_token_streq(js, t, "data")) {
                        state = DATA;
                    } else {
                        state = SKIP;
                        if (t->type == JSMN_ARRAY || t->type == JSMN_OBJECT) {
                            skip_tokens = t->size;
                        }
                    }
                }

                // Last object value
                if (node_tokens == 0) {
                    state = NODE_ARRAY;
                }

                break;

            case DATA:
                if (t->type != JSMN_ARRAY || t->type != JSMN_STRING) {
                    printf("Unknown data value: expected string or array.");
                }
                if (t->type == JSMN_ARRAY) {
                    elements = t->size;
                    printf("%lu elements\n", elements );
                    state = SKIP;
                    skip_tokens = elements;
                }

                break;

            case STOP:
                // Just consume the tokens
                break;

            default:
                printf("Invalid state %u", state);
        }
    }

    free(tokens);
    return (EXIT_SUCCESS);
}
