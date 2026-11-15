# Runtime-сущности от Operator и Adopt
Status: ready-for-human
Priority: 3

Слой E. Термины — `GLOSSARY.md` (Origin, Adopt).

- Operator в Operation создаёт/изменяет/удаляет runtime-Widgets (Origin=Runtime) на Scenes текущей Deployment; WidgetType менять не может.
- Все изменения — записи Journal.
- Adopt: Engineer явно принимает runtime-Tag или runtime-Widget в Draft (одна DraftChange), ID сохраняется; после Deploy сохранённого состояния Origin=Project.
- Регистрация runtime-Tag'ов со стороны Device: задача 34 закрыта, но Device Gateway пишет только значения уже развёрнутых Tag'ов (ADR 0004) — создание определений Tag'ов устройством остаётся здесь.
- Поле Origin (`Project`/`Runtime`) у Tag и Widget, enum без Unknown (ADR 0001); имя Tag уникально среди всех Tag проекта независимо от Origin. Перенесено из задачи 32 (ADR 0011): вводится вместе с первым создателем Runtime-origin сущностей.

## Comments

- 2026-10-04: бывший тикет 12 задачи 36. Решения: Q2 (runtime-сущности создают и Device/интеграции, и Operator; WidgetType меняет только Engineer), Q15 (runtime-Widget переживает Deploy), Q16 (Adopt с сохранением ID; имя Tag уникально независимо от Origin). Добавлена зависимость от задачи 38: Adopt — это DraftChange.
- 2026-11-15: сверено с кодом: поля `Origin` у Tag и Widget нет, Adopt отсутствует. Зависимости 38, 43 и 72_4 закрыты — строка `Blocked by` убрана.
