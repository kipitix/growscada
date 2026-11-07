# Агрегат User
Priority: 3
Blocked by: 31, 72_2

- User — член Organization (задача 31); Projects и Libraries принадлежат Organization, а не User.
- User становится автором Commit/Release/Deploy и носителем прав (задача 50); EditLock убран (72_1), поэтому «держателя замка» больше нет.

## Comments

- 2026-10-04: было «пользователю принадлежат библиотеки и проекты» — заменено решением grilling-сессии: владелец — Organization.
- 2026-11-07: EditLock убран; User становится автором Commit/Release/Deploy и прав. Зависимость от 72_2.
