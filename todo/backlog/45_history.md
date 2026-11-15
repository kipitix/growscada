# Режим History
Status: ready-for-human
Priority: 3

Слой D.

- Тот же Player, позиционируемый на любой момент прошлого, без управления процессом.
- Шкала времени, перемотка, проигрывание; лента событий Journal как побочный вид.
- Переключения работавшей Deployment (Deploy сохранённого состояния, Rollback) в прошлом отображаются корректно (Scenes и WidgetTypes той Deployment); для Deployment из сохранённого состояния показывать, какое состояние работало — `DeploymentResponse.commit_number`.
- Основа уже есть: `GET …/operation?at=<position>` отдаёт стартовое состояние из ближайшего Checkpoint (ADR 0023), а запись `deployment` Journal несёт `commit_number` (`contract/record/v0` MINOR 0.6). History — навигация и экран поверх них, новых таблиц не требует.

## Comments

- 2026-10-04: бывший тикет 10 задачи 36. Решение Q3: History = рабочая Scene в момент T (конфигурация + runtime-сущности + значения тегов); правки Draft в History не входят.
- 2026-11-07: Deployment теперь связана с сохранённым состоянием (`commit_number`, задача 85_4); в History показывать сохранённое состояние работавшей Deployment. Заблокирован 72_4.
- 2026-11-15: сверено с кодом. Зависимости 43 (Player), 44 (Checkpoint) и 72_4 закрыты — строка `Blocked by` убрана. `OperationAt` и `commit_number` в Checkpoint/Journal уже готовы; Release больше нет, «сохранённое состояние» — это saved state (`draft.Commit` в коде).
