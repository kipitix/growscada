# Undo/redo, Deploy, Operation и History

Status: ready-for-human

Проработка по итогам grilling-сессии 2026-09-26. Термины — в `CONTEXT.md` (разделы Roles, Nodes, Project lifecycle, Operation and History), архитектурное решение — `docs/adr/0002-engineering-runtime-split-journal-not-event-sourcing.md`.

## Цель

- Engineer редактирует проект в Draft с undo/redo, Draft сохраняется автоматически, в Operation изменения попадают только через Deploy.
- Operation и History работают от одного Player; History воспроизводит рабочую картину (конфигурация + runtime-сущности + значения тегов) на любой момент.
- Схема хранения событий простая: таблицы текущего состояния + один append-only Journal с «толстыми» записями + Checkpoints. Не event sourcing.

## Decisions so far

- Q1: Draft + Deploy, а не живое редактирование.
- Q2: Runtime-сущности (Tag, Widget) создают и Device/интеграции, и Operator в Operation. WidgetType меняет только Engineer — у WidgetType нет Origin.
- Q3: History = рабочая Scene в момент T (конфигурация + runtime-сущности + значения тегов); правки Draft в History не входят.
- Q4: Состояние в таблицах + Journal с полным новым состоянием в каждой записи.
- Q5: Один редактор; факт редактирования — EditLock (Acquire/Release).
- Q6: Роли людей — Engineer и Operator; блокировка — EditLock.
- Q7: Library (WidgetType) — часть Draft, уходит в Operation только через Deploy.
- Q8: Deploy публикует весь Draft атомарно → Revision.
- Q9: В Draft/Revision входит только определение Tag (имя, TagType); значение и Quality — только в runtime и Journal.
- Q10: Draft автосохраняется на сервере; кнопки «Сохранить» нет; явные действия — Deploy и Discard.
- Q11: Стек undo/redo — на сервере, у Draft.
- Q12: Стек живёт до Deploy/Discard; Release EditLock его не трогает; глубина ограничена константой.
- Q13: Шаг undo = одна DraftChange (одно намерение Engineer, может затрагивать несколько агрегатов).
- Q14: EditLock снимается по таймауту после отключения SSE-клиента; другой Engineer может перехватить с подтверждением.
- Q15: Runtime-Widget переживает Deploy; удаляется, только если исчезли его Scene или WidgetType (с записью в Journal и предупреждением перед Deploy).
- Q16: Adopt — явное принятие runtime-сущности в Draft с сохранением ID; имя Tag уникально независимо от Origin; Deploy с конфликтом имён отклоняется.
- Q17: Один Journal для всего, включая значения тегов.
- Q18: Operation и History — один Player.
- Q19: Checkpoint на каждый Deploy и периодически.
- Q20: Существующие таблицы становятся Draft; Revision — неизменяемый jsonb-документ.
- Q21: Один неявный проект (задача 29 отдельно).
- Q23–Q25: Два файла — ProjectFile (конфигурация) и PlaybackFile (Revisions + Journal + Checkpoints за период); PlaybackFile на сервере открывается только для просмотра в History.
- Q26–Q27: Один бинарник, роли Engineering node и Runtime node; сейчас обе в одном процессе.
- Q28: Две схемы БД (`engineering.*`, `runtime.*`), связь только через API Runtime node; номер Revision присваивает Runtime node.
- Q29: Открытый ProjectFile заменяет Draft одной DraftChange (требует EditLock).
- Q30: Rollback на Runtime node создаёт новую Revision со старым содержимым.

## Тикеты

| # | Тикет | Blocked by |
|---|-------|-----------|
| 01 | Разделение узлов и хранилищ, Tag definition/state, Origin | — |
| 02 | Толстые события и Journal | 01 |
| 03 | ProjectFile, Deploy, Revision, Discard | 01, 02 |
| 04 | EditLock | — |
| 05 | DraftChange и undo/redo | 03, 04 |
| 06 | ProjectFile в UI | 03, 05 |
| 07 | Rollback и список Revisions | 03 |
| 08 | Player и режим Operation | 02, 03 |
| 09 | Checkpoint | 02, 03 |
| 10 | Режим History | 08, 09 |
| 11 | PlaybackFile | 10 |
| 12 | Runtime-сущности от Operator и Adopt | 03, 08 |
| 13 | Разнесение узлов по сети (отложено) | 03 |

Связанные задачи вне пакета: 16 (версионирование JSON — формат ProjectFile/Journal), 20 (групповой перенос — после 05), 22 (Device регистрирует runtime-теги), 23 (поглощена тикетом 02), 29 (Project), 30/31 (User и права — кто может Deploy/Rollback).
