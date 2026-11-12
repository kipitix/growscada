# User settings are their own aggregate, stored apart from the content they view

The selection page of the Libraries (task 73) lists them in name order. An Engineer wants to arrange the list themselves, and the arrangement must outlive the tab: it belongs on the server, because the same person works from several devices. The obvious place — a `position` column on `engineering.libraries` — is the wrong one. The order is a fact about **N libraries at once**, so writing it would change N aggregates in one transaction, which ADR 0008 forbids; it would also raise every library's Version on every drag, so arranging the list would collide with renaming a library in another tab, and `growctl apply` would have to learn not to reset it. The same reasoning will apply to the Project list later.

Users do not exist yet (task 49) and authentication comes after them (task 50). Until then every request acts as one person, and the settings must be written in a shape that survives the arrival of real identities without a rewrite of the service, the API and the table's meaning.

Task 81. The term is `GLOSSARY.md` (User settings).

## Decision

- **A setting is an aggregate of its own, in `public`.** `public.user_settings` is keyed by `(user_id, organization_id, key)` with a `jsonb` payload and a Version: one row per person, Organization and setting, so a change of one is one transaction over one aggregate. The libraries keep their name, description and Version untouched; a drag and a rename never conflict.
- **`organization_id` from the first day.** The same person keeps a different order per Organization, so a second tenant starts from an empty one instead of inheriting the first one's view. It references `organizations`, the one table of the server's layer.
- **`user_id` has no foreign key yet, and the code names one person.** `settings.SingleUser()` is a fixed UUID every request acts as; the migration that adds the Users backfills those rows to the first User. The seam is the service: `NewSettingsService` takes the owner, and with authentication its owner comes from the request's principal. Neither the repository, nor the table, nor the API changes then.
- **The payload is a document with a typed codec per key.** The shape of `library_order` is `{"library_ids": [...]}`, encoded and decoded in `user_settings_codec`-style functions, as the Journal and the Checkpoints do. A second setting (theme, the last opened entity) is a new key with its own type and test — no migration, no column, no change of the aggregate's shape.
- **Ordering names libraries by ID, and tolerates them being gone.** No foreign key: the order is a view of the content, never an invariant about it. An ID of a deleted library is filtered out when the list is read and normalized away on the next write; a write never fails because a library was deleted in another tab. A duplicate and the zero ID are dropped, so one order has one representation.
- **No event and no outbox row.** Nothing else reacts to a preference, and the server's shared event stream is no place for one person's view — after authentication it would leak between people. The tab that wrote rereads on its own; another tab rereads on a resync. The precedent is the Tag state, which writes no outbox row (ADR 0011).
- **The client merges; the server stores.** `GET …/settings/library-order` answers the IDs the person placed, and the selection page puts them first, then the libraries the order does not name, by name. `PUT` sends the whole merged order the client shows. The settings service therefore never reads the libraries, `GET /libraries` keeps its creation order, and `growctl` (which shares that endpoint) never sees a personal arrangement.
- **A stale Version is 409, an unstated one is 0.** The Version is the usual optimistic token: 0 means no row yet (the first write inserts at 1), and a write at a Version the row no longer has is refused rather than overwriting a newer order from another tab. A second insert of one key is a conflict too.
- **A setting is never content.** No ProjectFile, Commit, Release or Deployment carries it, and no manifest kind describes it: canonical bytes and hashes do not change when someone arranges a list.

## Considered Options

- **`position` on `engineering.libraries`**: rejected. The order spans N aggregates, so one drag would be one transaction over all of them (ADR 0008), every drag would raise the libraries' Versions and collide with renames, and `growctl apply` would need a rule to preserve a value it does not own.
- **A dedicated `library_order` table** (`library_id`, `position`, `unique(position)` deferrable): rejected as the general mechanism. It needs a migration per setting, N row writes per drag and a deferrable unique constraint, while the same payload in one row is one statement; and the requirement is explicitly more per-user settings later.
- **Client-side order in LocalStorage**: rejected. It does not follow the person to another device or browser, which is the reason to store it at all.
- **Server-side merge in the settings endpoint**: rejected. It would make the setting's read depend on the libraries' aggregate, duplicate the list endpoint's job, and put a personal arrangement into a response `growctl` also reads.
- **A `library_order_changed` event in the outbox**: rejected. No aggregate reacts to a preference, the shared stream is not a per-person channel, and after authentication it would broadcast one person's view to everyone.
- **A foreign key to libraries with `ON DELETE CASCADE`**: rejected. It would either refuse deleting a library someone once arranged, or silently rewrite rows on a delete — deleting content is not a settings operation. Tolerating and pruning is simpler and never blocks a delete.
- **A settings aggregate without a Version**: rejected. Two tabs of one person are exactly as likely as two Engineers, and the last write would silently win.

## Consequences

- A migration in the `public` chain (`public.user_settings`), a new `domain/settings` package, a repository in `postgres/repositories`, `application.SettingsService`, and `GET`/`PUT /api/v0/settings/library-order` (`api/v0` MINOR 0.33 → 0.34, with the Bruno collection kept in step).
- The library selection page (task 82) reads the order and merges it with the list; the server keeps answering the list in creation order, and `growctl` is unaffected.
- A new setting is a key and a codec; the aggregate gains a method, not a column.
- Deleting a library leaves its ID in an order until the next write. That is by design: the order is a view, and a stale entry costs nothing.
- When Users arrive (task 49), the first migration backfills `SingleUser`'s rows to the first User; when authentication arrives (task 50), the settings service's owner becomes the request's principal. The table, the key and the API stay as they are.
- The settings are one row per person per Organization per key; they grow with the number of people, not with the content.
