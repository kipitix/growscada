---
description: Подготовить path к /kx_auto — чистое дерево, слияние спек, проверка очереди
---

Приведи репозиторий в состояние, с которого `/kx_auto` стартует: ветка `path`, чистое дерево, спецификации влиты, очередь задач видна. Это только подготовка: код не пиши, `/kx_auto` не запускай, не коммить, не пушь, уведомление в ntfy не отправляй.

Аргумент (`$ARGUMENTS`) — имя ветки или несколько через пробел, которые нужно влить в `path`. Без аргумента: текущая ветка, если это не `path`; если уже `path` — локальные ветки, которые добавляют спецификацию (`todo/backlog/*.md` со строкой `Parts:`, которой в `path` нет).

1. `git status --short`. Грязно — остановись и покажи, что мешает; ничего не коммить и не прятать.
2. Определи ветки для вливания:
   - с аргументом — возьми его;
   - без аргумента и текущая ветка не `path` — возьми текущую;
   - без аргумента и текущая `path` — найди ветки, которые строго впереди `path` и добавляют `Parts:`-файл, которого в `path` нет:

     ```bash
     for b in $(git for-each-ref --format='%(refname:short)' refs/heads); do
       [ "$b" = path ] && continue
       git merge-base --is-ancestor path "$b" || continue
       git merge-base --is-ancestor "$b" path && continue
       for f in $(git ls-tree -r --name-only "$b" -- todo/backlog | grep '\.md$'); do
         if git show "$b:$f" 2>/dev/null | grep -q '^Parts:' \
            && ! git show "path:$f" 2>/dev/null | grep -q '^Parts:'; then
           echo "$b"
           break
         fi
       done
     done
     ```

     Если таких нет — скажи, что вливать нечего.
3. `git switch path`.
4. Для каждой ветки:
   - `git merge-base --is-ancestor path <b>` — да: `git merge --ff-only <b>`;
   - нет (ветка разошлась с `path`): не ребейзить и не мержить коммитом; остановись и покажи ветку — решение за человеком.
5. Если влитая ветка меняла Go-код (`internal/`, `cmd/`, `contract/`, `schemas/`), прогони `make build` и `make test`. Красное — остановись, не продолжай подготовку.
6. Проверь, что `todo/backlog/` и `todo/done/` существуют.
7. Проверь очередь по скрипту из раздела «Выбор задачи» в `.opencode/commands/kx_auto.md` и покажи `NEXT` или `STOP`.
8. Покажи итог: какие ветки влиты, текущая ветка, `git status --short`, `NEXT`/`STOP`. Скажи, что дальше — `/kx_auto`.
