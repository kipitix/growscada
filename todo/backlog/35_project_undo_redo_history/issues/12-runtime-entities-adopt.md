# Runtime-сущности от Operator и Adopt

Type: task
Blocked by: 03, 08

- Operator в Operation создаёт/изменяет/удаляет runtime-Widgets (Origin=Runtime) на Scenes текущей Revision; WidgetType менять не может.
- Все изменения — записи Journal.
- Adopt: Engineer явно принимает runtime-Tag или runtime-Widget в Draft (одна DraftChange), ID сохраняется; после Deploy Origin=Project.
- Регистрация runtime-тегов со стороны Device — задача 22.
