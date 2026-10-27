# DraftChange и undo/redo
Status: ready-for-human
Blocked by: 35, 37

Слой C. Термин — `CONTEXT.md` (DraftChange).

- Каждое намерение Engineer — одна DraftChange: атомарно применяется к одному или нескольким агрегатам Draft (проекта или Library); сервер хранит состояние затронутых агрегатов «до» и «после».
- Undo/redo — серверные команды; стек переживает перезагрузку браузера и смену держателя EditLock.
- Стек очищается при Deploy, Release и Discard; глубина ограничена константой.
- Все текущие операции Project/Library переводятся на DraftChange (удаление Tag с привязками, удаление Scene с Widgets, смена закреплённого Library Release — одна DraftChange).
- UI: кнопки и Ctrl+Z / Ctrl+Shift+Z, описание следующего шага.
- После — задача 40 (групповой перенос как одна DraftChange).

## Comments

- 2026-10-04: бывший тикет 05 задачи 36. Решения: Q11 (стек undo/redo — на сервере, у Draft), Q12 (стек живёт до Deploy/Discard; Check in EditLock его не трогает; глубина ограничена), Q13 (шаг undo = одна DraftChange). Расширено: тот же механизм у Draft Library.
- 2026-10-06 (DDD-ревью задачи 20, ADR 0008): атомарная DraftChange над несколькими агрегатами требует Draft как агрегата (единственный писатель под EditLock). Тогда Scene, WidgetType и определения Tag — сущности внутри Draft, а не корни; пересмотреть механизмы задачи 20, рассчитанные на конкуренцию корней: CAS по каждой Scene, `retryOnRace`, политики по Scene в отдельных транзакциях. Правило «одна транзакция — один агрегат» остаётся, меняется размер агрегата.
- 2026-10-27 (grilling задачи 33, ADR 0012): с Draft-агрегатом снимаются переходные блокировки строк — Scene↔Library Pin (сироты) и WidgetType↔Library (Discard только для чтения); Discard Library выполняется одной транзакцией над Draft, без политики. Смена Library Pin — одна DraftChange.
- 2026-10-27 (код-ревью задачи 33): Release проверяет Version Library в своей транзакции (под блокировкой строки Library), но эта Version не покрывает Draft: запись WidgetType её не поднимает, поэтому Release может снять снимок Draft, которого клиент не видел (правка или удаление WidgetType между чтением клиента и Release). С Draft-агрегатом Release должен ожидать Version Draft.
