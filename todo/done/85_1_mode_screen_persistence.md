# Сохранять экран выбора Library/Project при переключении вкладок и перезагрузке

Status: ready-for-agent
Priority: 1
Parent: 85_labels_instead_of_releases
Part: 1/8

Слой D. Дефект найден при постановке задачи 85: переход Library → Project →
Library сбрасывает страницу выбора Library на редактор последней библиотеки.

## Проблема

`Root.SetMode` (`internal/server/interface/ui/root/root.go`) при каждом
открытии вкладки пересчитывает экран моды:

```go
if m := r.modeView(mode); m != nil && m.picker != nil {
    r.showList[mode] = r.selected[mode] == uuid.Nil
    ...
}
```

`showList` — это состояние экрана, а не выбор сущности: его ставят
`showListPage`, `openEntity`, `selectEntity` и `entityGone`. Пересчёт в
`SetMode` затирает его, поэтому человек, ушедший со страницы выбора (при уже
выбранной библиотеке), возвращается в редактор.

## Решение

- `SetMode` не трогает `showList` у уже открытой моды. Решение об экране
  принимается при первом открытии моды: `selected[mode] == uuid.Nil` →
  страница, иначе редактор.
- `showList` хранится в LocalStorage отдельно для Library и Project:
  `library:screen` и `project:screen` со значениями `list` / `editor`.
  `Root.OnMount` читает его до монтирования мод; отсутствие ключа означает
  старое поведение task 73 (есть выбранная сущность → редактор, нет →
  страница).
- `showListPage`, `openEntity`, `selectEntity` и `entityGone` обновляют и
  `showList`, и LocalStorage. `entityGone` всегда пишет `list`.
- Если сохранён `editor`, но выбранной сущности нет (`selected == uuid.Nil`),
  показывается страница; если сущность удалена и пришёл 404 —
  `entityGone` переводит в `list`.
- Повторное нажатие на активную вкладку не сбрасывает экран.
- `picker.Reload` при показе страницы сохраняется: страница перечитывает
  список, когда её показали (`showListPage`), при первом открытии моды и при
  восстановлении `list` из LocalStorage.
- `root:mode` не меняется: вкладка, открытая после перезагрузки, та же.

## Тесты

- `internal/server/interface/ui/root/selection_test.go` / `modes_test.go`:
  - редактор Library → Back → страница; Project → Library → страница;
  - редактор Library → Project → Library → редактор (если так было);
  - перезагрузка (повторный `OnMount`) с `library:screen=list` и выбранной
    библиотекой → страница; с `editor` → редактор;
  - `editor` без выбранной сущности → страница;
  - `entityGone` пишет `list` и перечитывает picker;
  - `showListPage` перечитывает picker ровно один раз.
- Ручной проход в PWA (правило 9): Library → Open → Back → Project → Library;
  страница осталась. Перезагрузить вкладку — страница осталась. То же для
  Project.

## Документы

- `AGENTS.md` (85_8): уточнить, что экран моды сохраняется в LocalStorage.

## Вне объёма

- Синхронизация экрана между вкладками браузера.
- Изменение вкладок Operation/History.

## Comments

- 2026-11-14: заведено из постановки 85, пункт 6.
- 2026-11-14 (грилинг): сохранять не только на время сессии, но и после
  перезагрузки; ключи `library:screen`/`project:screen`.
