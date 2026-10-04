# ProjectFile в UI
Status: ready-for-human
Blocked by: 29, 32

Слой C.

- Скачать Draft или любую Revision как ProjectFile.
- Открыть ProjectFile в Project: заменяет Draft одной DraftChange (undo возможен), требует EditLock.
- Загрузить ProjectFile на Runtime node вручную — это Deploy.
- Проверка `schema_version` и валидность содержимого до применения.

## Comments

- 2026-10-04: бывший тикет 06 задачи 36. Решения: Q23–Q25 (два файла — ProjectFile и PlaybackFile), Q29 (открытый ProjectFile заменяет Draft одной DraftChange, требует EditLock). Стоит после DraftChange (задача 32), поэтому перенесён из слоя B в слой C.
