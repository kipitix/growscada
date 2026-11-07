# Runtime node со вшитым Deployment (killer feature)
Priority: 3
Blocked by: 35, 72_4

- Из проекта (ProjectFile) собирается автономный исполняемый файл: Runtime node с уже вшитым работающим Deployment (тем, что сделан из Release), запускается на объекте без Engineering node и без Deploy.
- Решить: нужен ли такому бинарнику внешний Postgres или встроенное хранилище.

## Comments

- 2026-10-04: бывшая задача 20 «Сборка проекта в один исполняемый файл». Уточнено на grilling-сессии: речь о Runtime node со вшитым Deployment (сервер с WASM уже собирается в один бинарник).
