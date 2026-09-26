# ProjectFile в UI

Type: task
Blocked by: 03, 05

- Скачать Draft или любую Revision как ProjectFile.
- Открыть ProjectFile в Project: заменяет Draft одной DraftChange (undo возможен), требует EditLock.
- Загрузить ProjectFile на Runtime node вручную — это Deploy.
- Проверка `schema_version` и валидность содержимого до применения.
