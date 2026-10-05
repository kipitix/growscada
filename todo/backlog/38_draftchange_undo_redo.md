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

- 2026-10-04: бывший тикет 05 задачи 36. Решения: Q11 (стек undo/redo — на сервере, у Draft), Q12 (стек живёт до Deploy/Discard; Release EditLock его не трогает; глубина ограничена), Q13 (шаг undo = одна DraftChange). Расширено: тот же механизм у Draft Library.
