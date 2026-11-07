# Режим History
Status: ready-for-human
Priority: 3
Blocked by: 43, 44, 72_4

Слой D.

- Тот же Player, позиционируемый на любой момент прошлого, без управления процессом.
- Шкала времени, перемотка, проигрывание; лента событий Journal как побочный вид.
- Переключения работающей Deployment (Deploy Release/Rollback) в прошлом отображаются корректно (Scenes и WidgetTypes той Deployment); для Deployment с номером Release показывать, какая Release работала.

## Comments

- 2026-10-04: бывший тикет 10 задачи 36. Решение Q3: History = рабочая Scene в момент T (конфигурация + runtime-сущности + значения тегов); правки Draft в History не входят.
- 2026-11-07: Deployment теперь связана с Release (`release_number`, 72_4); в History показывать Release работавшей Deployment. Заблокирован 72_4.
