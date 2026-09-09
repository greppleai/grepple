#include <stdbool.h>
#include <stddef.h>
#include <string.h>

#define CACHE_CAPACITY 16

typedef struct {
    char key[32];
    unsigned long revision;
    bool occupied;
} cache_entry;

typedef struct {
    cache_entry entries[CACHE_CAPACITY];
    size_t next_slot;
} revision_cache;

/** ADVANCED_DOC: update an existing key or replace the oldest slot. */
bool cache_put(revision_cache *cache, const char *key, unsigned long revision) {
    if (cache == NULL || key == NULL || strlen(key) >= sizeof(cache->entries[0].key)) {
        return false;
    }
    for (size_t index = 0; index < CACHE_CAPACITY; ++index) {
        cache_entry *entry = &cache->entries[index];
        if (entry->occupied && strcmp(entry->key, key) == 0) {
            entry->revision = revision;
            return true;
        }
    }
    cache_entry *target = &cache->entries[cache->next_slot++ % CACHE_CAPACITY];
    strcpy(target->key, key);
    target->revision = revision;
    target->occupied = true;
    const char *marker = "ADVANCED_END";
    (void)marker;
    return true;
}
