# Поток сознания (разные мысли в процессе разработки)

## Индикация и команды

Есть очень хороший принцип [CQRS](https://ru.wikipedia.org/wiki/CQRS).
Думать похожим образом можно и в разрезе мнемосхем, которые пользователь использует в SCADA.

Что я имею ввиду?
CQRS - это Command Query Request Segregation.
Общая идея - разъединить сущности, которые отвечают за команды (изменение состояния) и запросы состояния.
На практике это выглядит так.
Если в HTTP запросе POST мы добавим новые данные в таблицу и отдадим результат SQL запроса обратно, затем на основании этого ответа обновим данные в UI, то всё будет работать, но с серьёзными ограничениями. Только один пользователь в 1 момент времени будет видеть актуальные данные. Будет сложнее оптимизировать запросы и работать с базой. Если же UI будет обновляться независимо от командных запросов (GET запрос и SELECT к БД), то указанные выше проблемы станут не актуальными.

Для SCADA проблема немного в другом.
Кажется интуитивным взять и работать в ПО строя модель цифрового двойника.
Создать тип некоторого прибора и определить его свойства и действия (как в ООП - аксессоры и мутаторы).
И потом из типа создавать инстансы.

Однако при разработке также удобно думать о некотором типе, который мы видим на экране и которым можем управлять.
Например есть какой-то прибор.
Он имеет состояние включен или отключен, а также может управляться включить или отключить.
И вот есть несколько таких приборов, и у одного из них появляется дополнительная команда сброс ошибок.
Или, например, другой случай, когда появляется вторая версия прибора и у него те же команды включить и отключить, но есть дополнительная индикация.
Вот и вопрос: как совместить удобство ООП и CQRS в части индикации и команд для элементов мнемосхем.

## Индикация в клиенте

Хочется использовать готовые изображения в SVG формате.
Нужно как-то получать данные из тегов и преобразовывать их в индикацию.
Для этого нужно будет писать логику.
Что-то вроде паттерна [MPV](https://en.wikipedia.org/wiki/Model–view–presenter).
Где Model - это теги, View - это SVG изображение, Presenter - это логика изменения вида.
Варианты реализации логики: Go, JavaScript, свой DSL язык.
Нужно оценить эти варианты, в первую очередь на возможность искажения логики.
Нужно быть уверенным, что будет закрыта возможность модификации логики.
Можно ли как-то использовать готовые инструменты для пошаговой отладки?

### Инициализация индикации

~~На момент инициализации индикационного элемента должны существовать теги, от которых зависит индикация.~~
Теги могут быть добавлены и после создания объекта, которых от них будет зависеть. Главное это обеспечить событиями о том, что тег создался и теперь надо обновить привязки.

### Индикация = SVG+JS

Типовой сценарий, чтобы что-то нарисовать на мнемосхеме:

1. Создать `WidgetType` в библиотеке (`Library`)
1.1. Создать `HTMLTemplate` - шаблон будущего виджета, показывающего индикацию
1.2. Создать `Script` который будет обновлять внешний вид виджета (`Widget`) на основании входных значений тегов (`Tag`), для скрипта нужно объявить входные теги и их типы
1.3. Путём подстановки значений тегов отладить изменение индикации, нужна возможность имитировать как сами значения (`TagValue`), так и качество тега (`TagQuality`)
2. Создать требуемые теги (`Tag`)
3. Расположить на сцене виджет (`Widget`), созданный на основании `WidgetType`
4. Осуществить привязку тегов (`Tag`) к виджету (`Widget`)
5. Открыть окно `Operation` и увидеть живую картину на мнемосхеме

Чтобы всё работало нужно учесть следующие моменты:
1. Где-то нужно хранить подписки на теги. При получении клиентом события обновления тега, нужно получить новое значение и перерисовать виджет.
2. Нужно где-то хранить скрипты в странице, желательно в обфусцированном виде или вообще в виде скрытом от пользователя (чтобы нельзя было увидеть скрипт через dev-tools), желательно избегать повторения вставки.
3. Должен быть вполне понятный способ искать в разметке часть элемента из `HTMLTemplate`. Проблема, которая может возникнуть - то, что на одной сцене будет несколько виджетов, созданных из одного `WidgetType`, а скрипт обновления удобнее, чтобы ориентировался на имена классов или ID из `HTMLTemplate`.
4. Желательно избежать дублирования не только скрипта обновления, но и самого виджета. Лучше всего было бы, если вставить в разметку один `HTMLTemplate` и дублировать его через что-то вроде `href` (не уверен, стоит уточнить),навесить на него подписки на теги и обновлять по событиям.

## Теги

Было два варианта реализации тегов: статическая типизация или динамическая типизация.

Я остановился на статической типизации.

Во-первых, это проще - должно вести себя предсказуемее, меньше дополнительных событий (например, когда тип изменился).

Во-вторых, можно извернуться и реализовать изменение типа через удаление и добавление тега с таким же именем.

## События

По DDD события - это объекты, которые фиксируют происхождение каких-то изменений состояния системы.
Они всегда имеют отметку времени и имеют имена в прошедшем времени.
Для удобства сначала создаётся внутренняя шина обмена событиями, а затем на уровне инфраструктуры шиша событий интегрируется с брокером событий.

Сценарии использования события:

- Обрабатывается запрос по HTTP, в прикладном сервисе вызывается метод, делается какое-то действие с агрегатом и после этого в шину событий отправляется событие.
- Событие должно быть сохранено для режима просмотра истории (получается по событиям должна быть возможность восстановить состояние системы на любой момент).

## Особенности реализации на Go

### Значения возвращаемые конструктором

Как-то попалась интересная [статья](https://duncanleung.com/go-idiom-accept-interfaces-return-types/) про такой подход. Если кратко, то "возвращайте из фабричных методов указатели на структуры, а при внедрении зависимостей принимайте интерфейсы".
Для DDD это может оказаться не всегда логично, особенно, если структура реализует один единственный интерфейс.
Если структура реализует несколько интерфейсов и её можно внедрить в несколько мест, то правило рабочее, но, это тоже может оказаться усложнение.
Для данного проекта буду придерживаться следующего подхода - писать интерфейс для Value Objects и Entities и возвращать его из фабричных методов.

Например VO TagValue:

```go
// NewTagValue creates a new tag value
func (t TagType) NewTagValue(aValue any) (TagValue, error) {
	switch t {
	case TagTypeString:
		return NewTagValueString(aValue)
	case TagTypeBoolean:
		return NewTagValueBoolean(aValue)
	case TagTypeInteger:
		return NewTagValueInteger(aValue)
	default:
		return nil, fmt.Errorf("unknown tag type: %v", t)
	}
}
```

В интерфейсе достаточно органично смотрится метод `Equals`, который принимает для сравнения другой TagID как интерфейс.

Также нужно обратить внимание на то, что методы использую value receivers, т.к. методы VO не должны менять его состояния.

### Репозитории, оптимистичный параллелизм и транзакции

В методе Save я решил одновременно использовать подход оптимистичного параллелизма без транзакций.

При оптимистичной блокировке оба запроса НЕ должны успешно выполниться. Это нормально, что второй запрос получает ошибку. Оптимистичная блокировка предполагает, что конфликты случаются редко, и приложение должно их обрабатывать (например, повторять операцию).

```go
// Ситуация: два пользователя одновременно обновляют один и тот же существующий тег
// Тег существует: ID=123, version=1, value="100"

// Goroutine A (обновляет value="200")
// Goroutine B (обновляет value="300")

// БЕЗ ТРАНЗАКЦИИ:
// A: SELECT version FROM tags WHERE id=123 → version=1
// B: SELECT version FROM tags WHERE id=123 → version=1
// A: UPDATE ... SET value="200", version=2 WHERE id=123 AND version=1 → успех (1 row)
// B: UPDATE ... SET value="300", version=2 WHERE id=123 AND version=1 → успех (0 rows - ошибка)
// Результат: B получил ErrOptimisticLock - ЭТО НОРМАЛЬНО!

// С ТРАНЗАКЦИЕЙ (READ COMMITTED) - без FOR UPDATE:
// A: BEGIN
// B: BEGIN
// A: SELECT version FROM tags WHERE id=123 → version=1
// B: SELECT version FROM tags WHERE id=123 → version=1 (ещё не видит изменения A)
// A: UPDATE ... SET value="200", version=2 WHERE id=123 AND version=1 → успех
// A: COMMIT
// B: UPDATE ... SET value="300", version=2 WHERE id=123 AND version=1 → 0 rows (версия уже 2)
// B: ROLLBACK (или COMMIT не делается)
// Результат: B получил ErrOptimisticLock - ТО ЖЕ САМОЕ!
```

Транзакция нужна не для борьбы с конкурентными обновлениями, а для другой проблемы:

```go
// Проблема: пользователь создаёт тег и сразу хочет его прочитать

// Goroutine A (создаёт тег)
// Goroutine B (читает тег)

// БЕЗ ТРАНЗАКЦИИ:
// A: SELECT EXISTS... → false
// B: SELECT * FROM tags WHERE id=123 → nil (ещё нет)
// A: INSERT INTO tags... → успех
// B: SELECT * FROM tags WHERE id=123 → тег уже есть
// Результат: B мог получить "тег не найден", хотя он уже создаётся

// С ТРАНЗАКЦИЕЙ (READ COMMITTED):
// A: BEGIN
// A: SELECT EXISTS... → false
// A: INSERT INTO tags... → успех (но другие транзакции не видят)
// B: SELECT * FROM tags WHERE id=123 → nil (транзакция A ещё не закоммичена)
// A: COMMIT
// B: SELECT * FROM tags WHERE id=123 → тег есть
// Результат: консистентность - B видит либо отсутствие, либо полное наличие
```

```go
// Проблема: гонка при создании тега с одним ID

// Goroutine A                           // Goroutine B
checkA: SELECT EXISTS... → false
                                        checkB: SELECT EXISTS... → false
insertA: INSERT... → успех
                                        insertB: INSERT... → ОШИБКА duplicate key

// БЕЗ ТРАНЗАКЦИИ:
// Ошибка duplicate key - это НЕ оптимистичная блокировка, а нарушение constraints
// Приложение получит sql.ErrNoRows? Нет, получит ошибку уникальности
// Нужно различать: ErrOptimisticLock vs ErrDuplicateKey

// С ТРАНЗАКЦИЕЙ и FOR UPDATE:
// A: BEGIN
// B: BEGIN
// A: SELECT ... FOR UPDATE → блокирует "гипотетическую" строку
// B: SELECT ... FOR UPDATE → ждёт
// A: INSERT... → успех
// A: COMMIT
// B: после COMMIT, SELECT видит, что строка существует, идёт на UPDATE
// Результат: B не получает duplicate key, а корректно обновляет
```

В принципе, даже без транзакции оптимистичный параллелизм защитит данные от искажения, но в коде мы не сможем отличить ErrOptimisticLock vs ErrDuplicateKey.

Для большей корректности и предсказуемости можно ввести транзакцию.

Можно попробовать обойтись без предварительного SELECT.

```go
// Оптимальный вариант - без лишней транзакции
func (r TagRepositoryPostgres) Save(ctx context.Context, t tag.Tag) error {
    if t.Version() == 0 {
        // Новый тег - просто INSERT
        // Если ID существует - получим ошибку уникальности
        _, err := r.db.ExecContext(ctx, `INSERT ...`, ...)
        return err
    }

    // Существующий тег - UPDATE с проверкой версии
    result, err := r.db.ExecContext(ctx, `
        UPDATE tags
        SET name=$1, value=$2, version=version+1
        WHERE id=$3 AND version=$4
    `, t.Name(), t.Value(), t.ID(), t.Version())

    if rows, _ := result.RowsAffected(); rows == 0 {
        return ErrOptimisticLock
    }
    return err
}
```

Один запрос всё же проще чем два!

## Анализ

### 2026-06-01 — Подготовка к режиму Operation и протоколу событий

#### 1. Главная проблема: нет именованных портов

`Widget.TagIDs []uuid.UUID` — просто массив без семантики. Скрипт не знает, какой тег — температура, а какой — давление. Нужно ввести понятие **входного порта**.

**Добавить `InputPorts` в `WidgetType`:**

```go
type InputPort struct {
    Name string   // "running", "speed" — ключ, который использует скрипт
    Type TagType  // tag.TagTypeBoolean / TagTypeInteger / TagTypeString
}
```

`WidgetType` получает поле `InputPorts() []InputPort`. Это контракт: что именно ждёт скрипт.

**Заменить `TagIDs` на `TagBindings` в `Widget`:**

```go
// Вместо: TagIDs() []id.ID[tag.Tag]
TagBindings() map[string]id.ID[tag.Tag]
// ключ = имя порта из WidgetType.InputPorts, значение = конкретный Tag ID
```

При получении события `TagUpdated` — найти все виджеты, подписанные на этот тег, и знать, под каким именем (`portName`) передать значение в скрипт.

#### 2. Конвенция Script и HTMLTemplate

**HTMLTemplate — только классы, без глобальных `id`:**

На одной сцене может быть несколько виджетов одного типа, `id` в DOM дублируется. Только классы:

```html
<div class="pump">
  <div class="pump__indicator"></div>
  <span class="pump__speed">--</span>
</div>
```

**Script — единый контракт `render(container, tagValues)`:**

```javascript
function render(container, tagValues) {
  // container: корневой DOM-элемент этого экземпляра виджета
  // tagValues: { portName: { value: string, quality: string } }

  const running = tagValues['running'];
  const indicator = container.querySelector('.pump__indicator');

  if (!running || running.quality === 'bad') {
    indicator.style.background = 'yellow';
  } else {
    indicator.style.background = running.value === 'true' ? 'green' : 'gray';
  }

  const speed = tagValues['speed'];
  if (speed && speed.quality === 'good') {
    container.querySelector('.pump__speed').textContent = speed.value;
  }
}
```

`container` изолирует один экземпляр виджета от другого на сцене.

#### 3. Хранение скриптов в DOM без дублирования

Для каждого `WidgetType` скрипт вставляется **один раз** и регистрируется в глобальном реестре:

```html
<script id="wts-<widgetTypeID>">
  (function() {
    window.__wt = window.__wt || {};
    window.__wt['<widgetTypeID>'] = function render(container, tagValues) {
      /* тело скрипта пользователя */
    };
  })();
</script>
```

При рендеринге Operation: если `window.__wt[wtID]` уже определён — не вставлять скрипт снова.

> Полностью скрыть скрипт от devtools невозможно — любой JS, выполняемый в браузере, виден. Минификация/обфускация — максимум что реально. Это ограничение платформы, не архитектуры.

#### 4. HTMLTemplate через `<template>` элемент — без дублирования разметки

```html
<!-- Один раз: шаблон WidgetType -->
<template id="wtmpl-<widgetTypeID>">
  <div class="pump">
    <div class="pump__indicator"></div>
    <span class="pump__speed">--</span>
  </div>
</template>

<!-- Для каждого экземпляра Widget: контейнер с клоном шаблона -->
<div data-widget-id="<widgetID>"
     data-widget-type="<widgetTypeID>"
     style="position:absolute; left:Xpx; top:Ypx; width:Wpx; height:Hpx;">
  <!-- сюда клонируется template.content -->
</div>
```

```javascript
const tmpl = document.getElementById('wtmpl-' + widgetTypeID);
const clone = tmpl.content.cloneNode(true);
container.appendChild(clone);
```

Стандартный [`<template>`](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/template) — браузерная фича именно для этого.

#### 5. Протокол передачи событий на Frontend — SSE

Server-Sent Events — однонаправленный поток сервер → клиент:

```
GET /api/v1/events
Accept: text/event-stream
```

Формат:

```
event: tag_updated
data: {"tag_id":"<uuid>","name":"pump_running","value":"true","quality":"good"}
```

На сервере: `restapi` подписывается на `EventBus.Subscribe(EventTypeTagUpdated, handler)` → пушит в SSE-клиента. Один goroutine на соединение, канал для передачи событий.

На клиенте (WASM):

```javascript
const es = new EventSource('/api/v1/events');
es.addEventListener('tag_updated', (e) => {
  const data = JSON.parse(e.data);
  updateWidgetsForTag(data.tag_id, data.value, data.quality);
});
```

#### 6. Реестр подписок на клиенте (Operation mode)

При загрузке сцены строится:

```
tagID → [ {widgetID, portName}, ... ]    // кого уведомить при обновлении тега
widgetID → { container, widgetTypeID, tagValues: { portName: {value, quality} } }
```

При событии `tag_updated`:
1. Найти все записи `tagID → widgets`
2. Для каждого `widgetID` → обновить `tagValues[portName]`
3. Вызвать `window.__wt[widgetTypeID](container, tagValues)`

#### 7. Улучшения Library для отладки (симуляция тегов)

Сейчас `editedInputData` — просто строка. Нужно:

- При объявлении `InputPorts` в WidgetType — показывать форму с полями по одному на порт
- Каждое поле: `value` (строка) + `quality` (select: good/bad/uncertain/simulated)
- `buildSrcdoc` собирает из этих полей объект `tagValues` и вызывает `render(container, tagValues)`

#### Итого: что нужно изменить

| Слой | Изменение |
|---|---|
| `domain/widget/WidgetType` | Добавить `InputPorts() []InputPort` |
| `domain/widget/Widget` | Заменить `TagIDs` → `TagBindings map[string]id.ID[tag.Tag]` |
| `appdto`, `restdto` | Добавить `input_ports`, заменить `tag_ids` на `tag_bindings` |
| DB migration | `input_ports jsonb` в `widget_types`, `tag_bindings jsonb` в `widgets` |
| REST API | Новый endpoint `GET /api/v1/events` (SSE) |
| UI Library | Форма симуляции с именованными портами (value + quality) |
| UI Operation | Клон `<template>`, реестр подписок, SSE-клиент, вызов `window.__wt[wtID]` |

