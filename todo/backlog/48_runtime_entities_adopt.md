# Runtime-сущности от Operator и Adopt
Status: ready-for-human
Blocked by: 38, 43

Слой E. Термины — `CONTEXT.md` (Origin, Adopt).

- Operator в Operation создаёт/изменяет/удаляет runtime-Widgets (Origin=Runtime) на Scenes текущей Revision; WidgetType менять не может.
- Все изменения — записи Journal.
- Adopt: Engineer явно принимает runtime-Tag или runtime-Widget в Draft (одна DraftChange), ID сохраняется; после Deploy Origin=Project.
- Регистрация runtime-тегов со стороны Device — после задачи 34.

## Comments

- 2026-10-04: бывший тикет 12 задачи 36. Решения: Q2 (runtime-сущности создают и Device/интеграции, и Operator; WidgetType меняет только Engineer), Q15 (runtime-Widget переживает Deploy), Q16 (Adopt с сохранением ID; имя Tag уникально независимо от Origin). Добавлена зависимость от задачи 38: Adopt — это DraftChange.
