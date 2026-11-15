# Runtime node со вшитым Deployment (killer feature)
Priority: 3

- Из проекта (ProjectFile) собирается автономный исполняемый файл: Runtime node с уже вшитым работающим Deployment (тем, что сделан из сохранённого состояния), запускается на объекте без Engineering node и без Deploy.
- Решить: нужен ли такому бинарнику внешний Postgres или встроенное хранилище.
- ProjectFile уже несёт весь конфиг объекта (Scenes, Widgets, Tags, Devices, Environments, импорты Library), а `POST …/projectfile/deploy` уже разворачивает файл на полном сервере; здесь — тот же Deployment без Engineering node.

## Comments

- 2026-10-04: бывшая задача 20 «Сборка проекта в один исполняемый файл». Уточнено на grilling-сессии: речь о Runtime node со вшитым Deployment (сервер с WASM уже собирается в один бинарник).
- 2026-11-15: сверено с кодом: `POST …/projectfile/deploy` и ProjectFile с Devices/Environments/Library Imports готовы (задача 34, 85_6). Зависимости 35 и 72_4 закрыты — `Blocked by` убран.
