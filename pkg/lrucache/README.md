# In-Memory LRU Cache for Golang Applications

[![](https://pkg.go.dev/badge/github.com/iamlouk/lrucache?utm_source=godoc)](https://pkg.go.dev/github.com/iamlouk/lrucache)

This library can be embedded into your existing go applications
and play the role *Memcached* or *Redis* might play for others.
It is inspired by [PHP Symfony's Cache Components](https://symfony.com/doc/current/components/cache/adapters/array_cache_adapter.html),
having a similar API. This library can not be used for persistance,
is not properly tested yet and a bit special in a few ways described
below (Especially with regards to the memory usage/`size`).

In addition to the interface described below, a `http.Handler` that can be used as middleware is provided as well.

- Advantages:
    - Anything (`interface{}`) can be stored as value
    - As it lives in the application itself, no serialization or de-serialization is needed
    - As it lives in the application itself, no memory moving/networking is needed
    - The computation of a new value for a key does __not__ block the full cache (only the key)
- Disadvantages:
    - You have to provide a size estimate for every value
    - __This size estimate should not change (i.e. values should not mutate)__
    - The cache can only be accessed by one application

## Example

```go
// Go look at the godocs and ./cache_test.go for more documentation and examples

maxMemory := 1000
cache := lrucache.New(maxMemory)

bar = cache.Get("foo", func () (value interface{}, ttl time.Duration, size int) {
	return "bar", 10 * time.Second, len("bar")
}).(string)

// bar == "bar"

bar = cache.Get("foo", func () (value interface{}, ttl time.Duration, size int) {
	panic("will not be called")
}).(string)
```

## Why does `cache.Get` take a function as argument?

*Using the mechanism described below is optional, the second argument to `Get` can be `nil` and there is a `Put` function as well.*

Because this library is meant to be used by multi threaded applications and the following would
result in the same data being fetched twice if both goroutines run in parallel:

```go
// This code shows what could happen with other cache libraries
c := lrucache.New(MAX_CACHE_ENTRIES)

for i := 0; i < 2; i++ {
    go func(){
        // This code will run twice in different goroutines,
        // it could overlap. As `fetchData` probably does some
        // I/O and takes a long time, the probability of both
        // goroutines calling `fetchData` is very high!
        url := "http://example.com/foo"
        contents := c.Get(url)
        if contents == nil {
            contents = fetchData(url)
            c.Set(url, contents)
        }

        handleData(contents.([]byte))
    }()
}

```

Here, if one wanted to make sure that only one of both goroutines fetches the data,
the programmer would need to build his own synchronization. That would suck!

```go
c := lrucache.New(MAX_CACHE_SIZE)

for i := 0; i < 2; i++ {
    go func(){
        url := "http://example.com/foo"
        contents := c.Get(url, func()(interface{}, time.Time, int) {
            // This closure will only be called once!
            // If another goroutine calls `c.Get` while this closure
            // is still being executed, it will wait.
            buf := fetchData(url)
            return buf, 100 * time.Second, len(buf)
        })

        handleData(contents.([]byte))
    }()
}

```

This is much better as less resources are wasted and synchronization is handled by
the library. If it gets called, the call to the closure happens synchronously. While
it is being executed, all other cache keys can still be accessed without having to wait
for the execution to be done.

## How `Get` works

The closure passed to `Get` will be called if the value asked for is not cached or
expired. It should return the following values:

- The value corresponding to that key and to be stored in the cache
- The time to live for that value (how long until it expires and needs to be recomputed)
- A size estimate

When `maxMemory` is reached, cache entries need to be evicted. Theoretically,
it would be possible to use reflection on every value placed in the cache
to get its exact size in bytes. This would be very expansive and slow though.
Also, size can change. Instead of this library calculating the size in bytes, you, the user,
have to provide a size for every value in whatever unit you like (as long as it is the same unit everywhere).

Suggestions on what to use as size: `len(str)` for strings, `len(slice) * size_of_slice_type`, etc.. It is possible
to use `1` as size for every entry, in that case at most `maxMemory` entries will be in the cache at the same time.

## Affects on GC

Because of the way a garbage collector decides when to run ([explained in the
runtime package](https://pkg.go.dev/runtime)), having large amounts of data
sitting in your cache might increase the memory consumption of your process by
two times the maximum size of the cache. You can decrease the *target
percentage* to reduce the effect, but then you might have negative performance
effects when your cache is not filled.
