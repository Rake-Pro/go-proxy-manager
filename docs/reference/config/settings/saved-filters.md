# Settings: saved filters

Named list filters for the admin UI, kept in settings so a filter saved on
one browser is offered on every other. They change nothing about how
requests are served.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-saved-filters"></span> `savedFilters` | []SavedFilter | At most 200 entries. Written by the UI's **Saved** menu, or by `PUT /api/settings/saved-filters` (scope `settings:write`), which replaces this section only. |

## SavedFilter (`savedFilters[]`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="settings-saved-filters-name"></span> `name` | string | none | yes | Shown in the Saved menu. Unique within its list, case-insensitively. At most 64 characters, one line. |
| <span id="settings-saved-filters-list"></span> `list` | string | none | yes | Which list the filter belongs to: `hosts`, `certificates`, `logs`, `history`, `tokens`, or an object list's id (`redirects`, `streams`, `parked`, `access`, `middleware`, `upstreams`, `dns`, `identity`, `clientcas`). Lower-case letters, digits and hyphens. |
| <span id="settings-saved-filters-query"></span> `query` | string | none | yes | The filter text as typed in the box, with the same grammar (words, `field:value`, a leading `-` to exclude, commas for alternatives). At most 1024 characters, one line. |

```yaml
savedFilters:
  - name: outside, errors only
    list: logs
    query: -client:@home-only -client:@uptimerobot -status:2xx,3xx
  - name: LAN hosts
    list: hosts
    query: tag:lan -status:disabled
```

- The UI also remembers the last filter typed on each list in the browser
  (`localStorage`, key `gpm.filter.<list>`) and puts it back when the list
  renders again; that memory is per browser and is not a setting.
- Saving or deleting a named filter commits settings, so it shows in History
  and can be reverted like any other change. A read-only role or an HA
  follower cannot save; the menu still applies what is stored.
