# PlaybackFile
Status: ready-for-human
Priority: 3
Blocked by: 45

Слой D. Термин — `GLOSSARY.md` (PlaybackFile).

- Выгрузка за период: Deployments, действовавшие в период (с номерами Release), записи Journal и необходимые Checkpoints.
- Загрузка на сервер открывает файл в History как отдельный источник воспроизведения — только просмотр, без смешивания с собственным Journal и без влияния на Operation.
- Восстановление сервера после аварии — не задача этого файла (бэкап БД).

## Comments

- 2026-10-04: бывший тикет 11 задачи 36. Решения Q23–Q25: PlaybackFile (Deployments + Journal + Checkpoints за период) на сервере открывается только для просмотра в History.
