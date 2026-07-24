-- Dynamic stat captions (spec section 14) for all 13 locales.
--
-- The caption engine already existed and worked, but content only covered ru-RU (30 phrases) and
-- en-US (10), across 4 of the 7 categories — so 11 of 13 locales showed a blank caption.
--
-- Each category now has ~4 phrases for each of the two parts in every locale, which combine into
-- ~16 distinct captions per category per language. Halves are only ever paired within the same
-- category, so an "overdue" state can never pick up a congratulatory second half.
--
-- The phrases are written idiomatically per language rather than translated from English, as the
-- spec requires ("свои естественные наборы, не машинный перевод"). No emoji: same product-wide
-- rule as the rest of the interface.
--
-- Existing ru-RU/en-US rows from 0002 are deleted first so those two locales don't end up with a
-- mix of old and new sets (which would skew the deterministic per-day pick toward the old ones).
DELETE FROM stat_captions WHERE locale IN ('ru-RU','en-US');

INSERT INTO stat_captions(locale, category, part, text) VALUES

-- ============================================================
-- en-US
-- ============================================================

-- en-US | success
('en-US', 'success', 1, 'Task crusher'),
('en-US', 'success', 1, 'On a roll'),
('en-US', 'success', 1, 'Deadline hunter'),
('en-US', 'success', 1, 'The closer'),
('en-US', 'success', 2, 'Keep that momentum going.'),
('en-US', 'success', 2, 'You''re firing on all cylinders.'),
('en-US', 'success', 2, 'Solid output this period.'),
('en-US', 'success', 2, 'The list is shrinking fast.'),

-- en-US | perfect_day
('en-US', 'perfect_day', 1, 'Clean slate'),
('en-US', 'perfect_day', 1, 'Nothing left behind'),
('en-US', 'perfect_day', 1, 'Flawless execution'),
('en-US', 'perfect_day', 1, 'Perfect run'),
('en-US', 'perfect_day', 2, 'Everything planned got done.'),
('en-US', 'perfect_day', 2, 'Not a single task missed.'),
('en-US', 'perfect_day', 2, 'That''s how it''s done.'),
('en-US', 'perfect_day', 2, 'Zero leftovers today.'),

-- en-US | overdue
('en-US', 'overdue', 1, 'Some tasks waiting'),
('en-US', 'overdue', 1, 'A few things pending'),
('en-US', 'overdue', 1, 'Items need attention'),
('en-US', 'overdue', 1, 'Backlog building up'),
('en-US', 'overdue', 2, 'Worth taking a look.'),
('en-US', 'overdue', 2, 'A few things slipped by.'),
('en-US', 'overdue', 2, 'Time to catch up.'),
('en-US', 'overdue', 2, 'Some tasks are past due.'),

-- en-US | inactive
('en-US', 'inactive', 1, 'Quiet period'),
('en-US', 'inactive', 1, 'Low activity lately'),
('en-US', 'inactive', 1, 'Taking it slow'),
('en-US', 'inactive', 1, 'Light work mode'),
('en-US', 'inactive', 2, 'Not much happened this period.'),
('en-US', 'inactive', 2, 'Activity has been minimal.'),
('en-US', 'inactive', 2, 'Things have slowed down.'),
('en-US', 'inactive', 2, 'Ready to pick up the pace?'),

-- en-US | focus
('en-US', 'focus', 1, 'Deep work mode'),
('en-US', 'focus', 1, 'In the zone'),
('en-US', 'focus', 1, 'Focused and steady'),
('en-US', 'focus', 1, 'Flow state regular'),
('en-US', 'focus', 2, 'Distraction-free work pays off.'),
('en-US', 'focus', 2, 'Long focus sessions logged.'),
('en-US', 'focus', 2, 'Quality over quantity here.'),
('en-US', 'focus', 2, 'Deep work is working.'),

-- en-US | project
('en-US', 'project', 1, 'Project mover'),
('en-US', 'project', 1, 'Steady builder'),
('en-US', 'project', 1, 'Moving the needle'),
('en-US', 'project', 1, 'Progress driver'),
('en-US', 'project', 2, 'The project is moving forward.'),
('en-US', 'project', 2, 'Good progress on the list.'),
('en-US', 'project', 2, 'Steady steps add up.'),
('en-US', 'project', 2, 'Closer to the finish line.'),

-- en-US | neutral
('en-US', 'neutral', 1, 'Steady as it goes'),
('en-US', 'neutral', 1, 'Holding the line'),
('en-US', 'neutral', 1, 'Consistent effort'),
('en-US', 'neutral', 1, 'Business as usual'),
('en-US', 'neutral', 2, 'Nothing unusual to report.'),
('en-US', 'neutral', 2, 'A typical, steady period.'),
('en-US', 'neutral', 2, 'Things are moving along.'),
('en-US', 'neutral', 2, 'Consistent work continues.'),

-- ============================================================
-- ru-RU
-- ============================================================

-- ru-RU | success
('ru-RU', 'success', 1, 'Машина продуктивности'),
('ru-RU', 'success', 1, 'Герой дедлайнов'),
('ru-RU', 'success', 1, 'Тихий терминатор задач'),
('ru-RU', 'success', 1, 'Повелитель чекбоксов'),
('ru-RU', 'success', 2, 'Хороший темп.'),
('ru-RU', 'success', 2, 'Список тает на глазах.'),
('ru-RU', 'success', 2, 'Так держать.'),
('ru-RU', 'success', 2, 'Продуктивный период позади.'),

-- ru-RU | perfect_day
('ru-RU', 'perfect_day', 1, 'Идеальный день'),
('ru-RU', 'perfect_day', 1, 'Ни одной задачи мимо'),
('ru-RU', 'perfect_day', 1, 'Чистый лист'),
('ru-RU', 'perfect_day', 1, 'Без единого хвоста'),
('ru-RU', 'perfect_day', 2, 'Всё по плану выполнено.'),
('ru-RU', 'perfect_day', 2, 'Ни одна задача не ускользнула.'),
('ru-RU', 'perfect_day', 2, 'Вот как это делается.'),
('ru-RU', 'perfect_day', 2, 'День закрыт чисто.'),

-- ru-RU | overdue
('ru-RU', 'overdue', 1, 'Есть просроченные задачи'),
('ru-RU', 'overdue', 1, 'Кое-что ждёт'),
('ru-RU', 'overdue', 1, 'Хвосты накапливаются'),
('ru-RU', 'overdue', 1, 'Задачи требуют внимания'),
('ru-RU', 'overdue', 2, 'Стоит заглянуть в список.'),
('ru-RU', 'overdue', 2, 'Несколько задач просрочено.'),
('ru-RU', 'overdue', 2, 'Время разобраться с хвостами.'),
('ru-RU', 'overdue', 2, 'Пора наверстать.'),

-- ru-RU | inactive
('ru-RU', 'inactive', 1, 'Тихий период'),
('ru-RU', 'inactive', 1, 'Низкая активность'),
('ru-RU', 'inactive', 1, 'Замедленный режим'),
('ru-RU', 'inactive', 1, 'Работа на паузе'),
('ru-RU', 'inactive', 2, 'Активность была минимальной.'),
('ru-RU', 'inactive', 2, 'Почти ничего не произошло.'),
('ru-RU', 'inactive', 2, 'Всё затихло на время.'),
('ru-RU', 'inactive', 2, 'Готов снова разогнаться?'),

-- ru-RU | focus
('ru-RU', 'focus', 1, 'В потоке'),
('ru-RU', 'focus', 1, 'Режим глубокой работы'),
('ru-RU', 'focus', 1, 'Сосредоточен и спокоен'),
('ru-RU', 'focus', 1, 'Мастер концентрации'),
('ru-RU', 'focus', 2, 'Долгие сессии без отвлечений.'),
('ru-RU', 'focus', 2, 'Глубокая работа приносит плоды.'),
('ru-RU', 'focus', 2, 'Качество важнее количества.'),
('ru-RU', 'focus', 2, 'Фокус работает.'),

-- ru-RU | project
('ru-RU', 'project', 1, 'Двигатель проекта'),
('ru-RU', 'project', 1, 'Строитель результата'),
('ru-RU', 'project', 1, 'Прогресс очевиден'),
('ru-RU', 'project', 1, 'Проект в движении'),
('ru-RU', 'project', 2, 'Проект идёт вперёд.'),
('ru-RU', 'project', 2, 'Хорошие результаты по списку.'),
('ru-RU', 'project', 2, 'Шаг за шагом — к цели.'),
('ru-RU', 'project', 2, 'Финиш всё ближе.'),

-- ru-RU | neutral
('ru-RU', 'neutral', 1, 'Стабильность — признак мастерства'),
('ru-RU', 'neutral', 1, 'Работа идёт'),
('ru-RU', 'neutral', 1, 'Ровный ритм'),
('ru-RU', 'neutral', 1, 'Без сюрпризов'),
('ru-RU', 'neutral', 2, 'Всё идёт своим чередом.'),
('ru-RU', 'neutral', 2, 'Обычный рабочий период.'),
('ru-RU', 'neutral', 2, 'Ничего особенного.'),
('ru-RU', 'neutral', 2, 'Стабильный результат.'),

-- ============================================================
-- uk-UA
-- ============================================================

-- uk-UA | success
('uk-UA', 'success', 1, 'Машина продуктивності'),
('uk-UA', 'success', 1, 'Мисливець на дедлайни'),
('uk-UA', 'success', 1, 'Руйнівник списків'),
('uk-UA', 'success', 1, 'Король завдань'),
('uk-UA', 'success', 2, 'Чудовий темп цього разу.'),
('uk-UA', 'success', 2, 'Список тане на очах.'),
('uk-UA', 'success', 2, 'Так тримати.'),
('uk-UA', 'success', 2, 'Продуктивний період.'),

-- uk-UA | perfect_day
('uk-UA', 'perfect_day', 1, 'Ідеальний день'),
('uk-UA', 'perfect_day', 1, 'Чиста сторінка'),
('uk-UA', 'perfect_day', 1, 'Жодного хвоста'),
('uk-UA', 'perfect_day', 1, 'Все за планом'),
('uk-UA', 'perfect_day', 2, 'Усі завдання виконано.'),
('uk-UA', 'perfect_day', 2, 'Жодного пропуску.'),
('uk-UA', 'perfect_day', 2, 'Ось як це робиться.'),
('uk-UA', 'perfect_day', 2, 'День закрито чисто.'),

-- uk-UA | overdue
('uk-UA', 'overdue', 1, 'Є прострочені завдання'),
('uk-UA', 'overdue', 1, 'Дещо очікує'),
('uk-UA', 'overdue', 1, 'Хвости накопичились'),
('uk-UA', 'overdue', 1, 'Завдання потребують уваги'),
('uk-UA', 'overdue', 2, 'Варто зазирнути до списку.'),
('uk-UA', 'overdue', 2, 'Кілька завдань прострочено.'),
('uk-UA', 'overdue', 2, 'Час надолужити.'),
('uk-UA', 'overdue', 2, 'Деякі пункти чекають.'),

-- uk-UA | inactive
('uk-UA', 'inactive', 1, 'Тихий період'),
('uk-UA', 'inactive', 1, 'Мала активність'),
('uk-UA', 'inactive', 1, 'Сповільнений режим'),
('uk-UA', 'inactive', 1, 'Робота на паузі'),
('uk-UA', 'inactive', 2, 'Активність була мінімальною.'),
('uk-UA', 'inactive', 2, 'Майже нічого не відбулось.'),
('uk-UA', 'inactive', 2, 'Все затихло на час.'),
('uk-UA', 'inactive', 2, 'Готовий знову набрати темп?'),

-- uk-UA | focus
('uk-UA', 'focus', 1, 'У потоці'),
('uk-UA', 'focus', 1, 'Режим глибокої роботи'),
('uk-UA', 'focus', 1, 'Зосереджений і спокійний'),
('uk-UA', 'focus', 1, 'Майстер концентрації'),
('uk-UA', 'focus', 2, 'Довгі сесії без відволікань.'),
('uk-UA', 'focus', 2, 'Глибока робота дає результат.'),
('uk-UA', 'focus', 2, 'Якість важливіша за кількість.'),
('uk-UA', 'focus', 2, 'Фокус справді працює.'),

-- uk-UA | project
('uk-UA', 'project', 1, 'Двигун проекту'),
('uk-UA', 'project', 1, 'Будівник результату'),
('uk-UA', 'project', 1, 'Прогрес очевидний'),
('uk-UA', 'project', 1, 'Проект у русі'),
('uk-UA', 'project', 2, 'Проект рухається вперед.'),
('uk-UA', 'project', 2, 'Добрі результати за списком.'),
('uk-UA', 'project', 2, 'Крок за кроком — до мети.'),
('uk-UA', 'project', 2, 'Фінал все ближче.'),

-- uk-UA | neutral
('uk-UA', 'neutral', 1, 'Стабільний ритм'),
('uk-UA', 'neutral', 1, 'Робота йде'),
('uk-UA', 'neutral', 1, 'Без сюрпризів'),
('uk-UA', 'neutral', 1, 'Рівний темп'),
('uk-UA', 'neutral', 2, 'Все йде своїм чередом.'),
('uk-UA', 'neutral', 2, 'Звичайний робочий період.'),
('uk-UA', 'neutral', 2, 'Нічого особливого.'),
('uk-UA', 'neutral', 2, 'Стабільний результат.'),

-- ============================================================
-- be-BY
-- ============================================================

-- be-BY | success
('be-BY', 'success', 1, 'Машына прадуктыўнасці'),
('be-BY', 'success', 1, 'Паляўнічы на тэрміны'),
('be-BY', 'success', 1, 'Разбуральнік спісаў'),
('be-BY', 'success', 1, 'Гаспадар задач'),
('be-BY', 'success', 2, 'Добры тэмп на гэты раз.'),
('be-BY', 'success', 2, 'Спіс тае на вачах.'),
('be-BY', 'success', 2, 'Так трымаць.'),
('be-BY', 'success', 2, 'Прадуктыўны перыяд.'),

-- be-BY | perfect_day
('be-BY', 'perfect_day', 1, 'Ідэальны дзень'),
('be-BY', 'perfect_day', 1, 'Чысты аркуш'),
('be-BY', 'perfect_day', 1, 'Без ніводнага хваста'),
('be-BY', 'perfect_day', 1, 'Усё па плане'),
('be-BY', 'perfect_day', 2, 'Усе задачы выкананы.'),
('be-BY', 'perfect_day', 2, 'Ніводнага прапуску.'),
('be-BY', 'perfect_day', 2, 'Вось як гэта робіцца.'),
('be-BY', 'perfect_day', 2, 'Дзень зачынены чыста.'),

-- be-BY | overdue
('be-BY', 'overdue', 1, 'Ёсць затрыманыя задачы'),
('be-BY', 'overdue', 1, 'Нешта чакае'),
('be-BY', 'overdue', 1, 'Хвасты назапасіліся'),
('be-BY', 'overdue', 1, 'Задачы патрабуюць увагі'),
('be-BY', 'overdue', 2, 'Варта зірнуць у спіс.'),
('be-BY', 'overdue', 2, 'Некалькі задач затрымана.'),
('be-BY', 'overdue', 2, 'Час разабрацца з хвастамі.'),
('be-BY', 'overdue', 2, 'Пара наверстаць.'),

-- be-BY | inactive
('be-BY', 'inactive', 1, 'Ціхі перыяд'),
('be-BY', 'inactive', 1, 'Нізкая актыўнасць'),
('be-BY', 'inactive', 1, 'Запаволены рэжым'),
('be-BY', 'inactive', 1, 'Праца на паўзе'),
('be-BY', 'inactive', 2, 'Актыўнасць была мінімальнай.'),
('be-BY', 'inactive', 2, 'Амаль нічога не адбылося.'),
('be-BY', 'inactive', 2, 'Усё сціхла на час.'),
('be-BY', 'inactive', 2, 'Гатовы зноў набраць тэмп?'),

-- be-BY | focus
('be-BY', 'focus', 1, 'У патоку'),
('be-BY', 'focus', 1, 'Рэжым глыбокай працы'),
('be-BY', 'focus', 1, 'Засяроджаны і спакойны'),
('be-BY', 'focus', 1, 'Майстар канцэнтрацыі'),
('be-BY', 'focus', 2, 'Доўгія сесіі без адцягненняў.'),
('be-BY', 'focus', 2, 'Глыбокая праца прыносіць плады.'),
('be-BY', 'focus', 2, 'Якасць важнейшая за колькасць.'),
('be-BY', 'focus', 2, 'Фокус сапраўды працуе.'),

-- be-BY | project
('be-BY', 'project', 1, 'Рухавік праекта'),
('be-BY', 'project', 1, 'Будаўнік выніку'),
('be-BY', 'project', 1, 'Прагрэс відавочны'),
('be-BY', 'project', 1, 'Праект у руху'),
('be-BY', 'project', 2, 'Праект рухаецца наперад.'),
('be-BY', 'project', 2, 'Добрыя вынікі па спісе.'),
('be-BY', 'project', 2, 'Крок за крокам да мэты.'),
('be-BY', 'project', 2, 'Фінал усё бліжэй.'),

-- be-BY | neutral
('be-BY', 'neutral', 1, 'Стабільны рытм'),
('be-BY', 'neutral', 1, 'Праца ідзе'),
('be-BY', 'neutral', 1, 'Без сюрпрызаў'),
('be-BY', 'neutral', 1, 'Роўны тэмп'),
('be-BY', 'neutral', 2, 'Усё ідзе сваім чарадом.'),
('be-BY', 'neutral', 2, 'Звычайны рабочы перыяд.'),
('be-BY', 'neutral', 2, 'Нічога асаблівага.'),
('be-BY', 'neutral', 2, 'Стабільны вынік.'),

-- ============================================================
-- kk-KZ
-- ============================================================

-- kk-KZ | success
('kk-KZ', 'success', 1, 'Тапсырмалар жеңімпазы'),
('kk-KZ', 'success', 1, 'Мерзімдер аңшысы'),
('kk-KZ', 'success', 1, 'Тізім жойғышы'),
('kk-KZ', 'success', 1, 'Жемісті кезең'),
('kk-KZ', 'success', 2, 'Жақсы қарқын сақталды.'),
('kk-KZ', 'success', 2, 'Тізім тез қысқарды.'),
('kk-KZ', 'success', 2, 'Осылай жалғастыр.'),
('kk-KZ', 'success', 2, 'Өнімді кезең аяқталды.'),

-- kk-KZ | perfect_day
('kk-KZ', 'perfect_day', 1, 'Мінсіз күн'),
('kk-KZ', 'perfect_day', 1, 'Таза бет'),
('kk-KZ', 'perfect_day', 1, 'Жоспар орындалды'),
('kk-KZ', 'perfect_day', 1, 'Ештеңе қалмады'),
('kk-KZ', 'perfect_day', 2, 'Барлық тапсырма орындалды.'),
('kk-KZ', 'perfect_day', 2, 'Бірде-бір тапсырма өткізілмеді.'),
('kk-KZ', 'perfect_day', 2, 'Міне, дұрыс жол.'),
('kk-KZ', 'perfect_day', 2, 'Күн таза жабылды.'),

-- kk-KZ | overdue
('kk-KZ', 'overdue', 1, 'Мерзімі өткен тапсырмалар'),
('kk-KZ', 'overdue', 1, 'Кейбір нәрселер күтеді'),
('kk-KZ', 'overdue', 1, 'Артта қалғандар бар'),
('kk-KZ', 'overdue', 1, 'Назар аудару керек'),
('kk-KZ', 'overdue', 2, 'Тізімге қарап шығу керек.'),
('kk-KZ', 'overdue', 2, 'Бірнеше тапсырма кешіктірілді.'),
('kk-KZ', 'overdue', 2, 'Жетіспеушілікті толтыру уақыты.'),
('kk-KZ', 'overdue', 2, 'Кейбір тапсырмалар өтіп кетті.'),

-- kk-KZ | inactive
('kk-KZ', 'inactive', 1, 'Тыныш кезең'),
('kk-KZ', 'inactive', 1, 'Белсенділік төмен'),
('kk-KZ', 'inactive', 1, 'Баяу режим'),
('kk-KZ', 'inactive', 1, 'Жұмыс үзілісте'),
('kk-KZ', 'inactive', 2, 'Белсенділік өте аз болды.'),
('kk-KZ', 'inactive', 2, 'Бұл кезеңде аз нәрсе болды.'),
('kk-KZ', 'inactive', 2, 'Бәрі біраз тынышталды.'),
('kk-KZ', 'inactive', 2, 'Қайта қарқын алуға дайынсыз ба?'),

-- kk-KZ | focus
('kk-KZ', 'focus', 1, 'Терең жұмыс режимі'),
('kk-KZ', 'focus', 1, 'Ағымда'),
('kk-KZ', 'focus', 1, 'Шоғырланған және сабырлы'),
('kk-KZ', 'focus', 1, 'Зейін шебері'),
('kk-KZ', 'focus', 2, 'Ұзақ сессиялар жазылды.'),
('kk-KZ', 'focus', 2, 'Терең жұмыс нәтиже береді.'),
('kk-KZ', 'focus', 2, 'Сапа санынан маңызды.'),
('kk-KZ', 'focus', 2, 'Зейін жұмыс жасайды.'),

-- kk-KZ | project
('kk-KZ', 'project', 1, 'Жоба қозғаушысы'),
('kk-KZ', 'project', 1, 'Тұрақты құрастырушы'),
('kk-KZ', 'project', 1, 'Ілгерілеу байқалады'),
('kk-KZ', 'project', 1, 'Жоба қозғалыста'),
('kk-KZ', 'project', 2, 'Жоба алға жылжып жатыр.'),
('kk-KZ', 'project', 2, 'Тізім бойынша жақсы нәтиже.'),
('kk-KZ', 'project', 2, 'Қадам-қадам мақсатқа.'),
('kk-KZ', 'project', 2, 'Финал жақындап келеді.'),

-- kk-KZ | neutral
('kk-KZ', 'neutral', 1, 'Тұрақты ырғақ'),
('kk-KZ', 'neutral', 1, 'Жұмыс жүріп жатыр'),
('kk-KZ', 'neutral', 1, 'Тосынсыз'),
('kk-KZ', 'neutral', 1, 'Қалыпты режим'),
('kk-KZ', 'neutral', 2, 'Бәрі өз жолымен жүруде.'),
('kk-KZ', 'neutral', 2, 'Қарапайым жұмыс кезеңі.'),
('kk-KZ', 'neutral', 2, 'Ерекше ештеңе жоқ.'),
('kk-KZ', 'neutral', 2, 'Тұрақты нәтиже.'),

-- ============================================================
-- es-ES
-- ============================================================

-- es-ES | success
('es-ES', 'success', 1, 'Tritura tareas'),
('es-ES', 'success', 1, 'Cazador de plazos'),
('es-ES', 'success', 1, 'A pleno rendimiento'),
('es-ES', 'success', 1, 'Imparable esta semana'),
('es-ES', 'success', 2, 'Buen ritmo, sigue así.'),
('es-ES', 'success', 2, 'La lista se va vaciando.'),
('es-ES', 'success', 2, 'Período muy productivo.'),
('es-ES', 'success', 2, 'Así se hace.'),

-- es-ES | perfect_day
('es-ES', 'perfect_day', 1, 'Día perfecto'),
('es-ES', 'perfect_day', 1, 'Sin pendientes'),
('es-ES', 'perfect_day', 1, 'Pizarra limpia'),
('es-ES', 'perfect_day', 1, 'Todo resuelto'),
('es-ES', 'perfect_day', 2, 'Todo lo planeado, completado.'),
('es-ES', 'perfect_day', 2, 'Ni una tarea escapó.'),
('es-ES', 'perfect_day', 2, 'Así es como se hace.'),
('es-ES', 'perfect_day', 2, 'Jornada cerrada a cero.'),

-- es-ES | overdue
('es-ES', 'overdue', 1, 'Tareas pendientes'),
('es-ES', 'overdue', 1, 'Algunas cosas esperan'),
('es-ES', 'overdue', 1, 'Atrasos acumulados'),
('es-ES', 'overdue', 1, 'Pendientes por revisar'),
('es-ES', 'overdue', 2, 'Merece echar un vistazo.'),
('es-ES', 'overdue', 2, 'Algunas tareas se han retrasado.'),
('es-ES', 'overdue', 2, 'Momento de ponerse al día.'),
('es-ES', 'overdue', 2, 'Hay cosas fuera de plazo.'),

-- es-ES | inactive
('es-ES', 'inactive', 1, 'Período tranquilo'),
('es-ES', 'inactive', 1, 'Poca actividad reciente'),
('es-ES', 'inactive', 1, 'Ritmo suave'),
('es-ES', 'inactive', 1, 'Modo reposo'),
('es-ES', 'inactive', 2, 'Poca actividad este período.'),
('es-ES', 'inactive', 2, 'Las cosas han ido despacio.'),
('es-ES', 'inactive', 2, 'Casi nada en este intervalo.'),
('es-ES', 'inactive', 2, 'Listo para retomar el ritmo?'),

-- es-ES | focus
('es-ES', 'focus', 1, 'Modo concentración'),
('es-ES', 'focus', 1, 'En estado de flujo'),
('es-ES', 'focus', 1, 'Enfocado y sereno'),
('es-ES', 'focus', 1, 'Trabajo profundo'),
('es-ES', 'focus', 2, 'Sesiones largas sin distracciones.'),
('es-ES', 'focus', 2, 'El trabajo profundo da frutos.'),
('es-ES', 'focus', 2, 'Calidad antes que cantidad.'),
('es-ES', 'focus', 2, 'La concentración funciona.'),

-- es-ES | project
('es-ES', 'project', 1, 'Motor del proyecto'),
('es-ES', 'project', 1, 'Constructor constante'),
('es-ES', 'project', 1, 'Avance visible'),
('es-ES', 'project', 1, 'Proyecto en marcha'),
('es-ES', 'project', 2, 'El proyecto avanza bien.'),
('es-ES', 'project', 2, 'Buen progreso en la lista.'),
('es-ES', 'project', 2, 'Paso a paso hacia la meta.'),
('es-ES', 'project', 2, 'El final está más cerca.'),

-- es-ES | neutral
('es-ES', 'neutral', 1, 'Ritmo constante'),
('es-ES', 'neutral', 1, 'Sin novedades'),
('es-ES', 'neutral', 1, 'Esfuerzo sostenido'),
('es-ES', 'neutral', 1, 'Un período normal'),
('es-ES', 'neutral', 2, 'Todo sigue su curso.'),
('es-ES', 'neutral', 2, 'Un período sin sobresaltos.'),
('es-ES', 'neutral', 2, 'Trabajo constante y estable.'),
('es-ES', 'neutral', 2, 'Nada especial que destacar.'),

-- ============================================================
-- pt-BR
-- ============================================================

-- pt-BR | success
('pt-BR', 'success', 1, 'Destruidor de tarefas'),
('pt-BR', 'success', 1, 'Caçador de prazos'),
('pt-BR', 'success', 1, 'No ritmo certo'),
('pt-BR', 'success', 1, 'Produtividade no topo'),
('pt-BR', 'success', 2, 'Bom ritmo, continua assim.'),
('pt-BR', 'success', 2, 'A lista está diminuindo.'),
('pt-BR', 'success', 2, 'Período bastante produtivo.'),
('pt-BR', 'success', 2, 'É assim que se faz.'),

-- pt-BR | perfect_day
('pt-BR', 'perfect_day', 1, 'Dia perfeito'),
('pt-BR', 'perfect_day', 1, 'Sem pendências'),
('pt-BR', 'perfect_day', 1, 'Tudo resolvido'),
('pt-BR', 'perfect_day', 1, 'Lousa limpa'),
('pt-BR', 'perfect_day', 2, 'Tudo planejado foi feito.'),
('pt-BR', 'perfect_day', 2, 'Nenhuma tarefa ficou para trás.'),
('pt-BR', 'perfect_day', 2, 'É assim que funciona.'),
('pt-BR', 'perfect_day', 2, 'Dia encerrado no zero.'),

-- pt-BR | overdue
('pt-BR', 'overdue', 1, 'Tarefas atrasadas'),
('pt-BR', 'overdue', 1, 'Algumas coisas esperando'),
('pt-BR', 'overdue', 1, 'Acúmulo de pendências'),
('pt-BR', 'overdue', 1, 'Itens para revisar'),
('pt-BR', 'overdue', 2, 'Vale dar uma olhada.'),
('pt-BR', 'overdue', 2, 'Algumas tarefas estão atrasadas.'),
('pt-BR', 'overdue', 2, 'Hora de se atualizar.'),
('pt-BR', 'overdue', 2, 'Tem coisa fora do prazo.'),

-- pt-BR | inactive
('pt-BR', 'inactive', 1, 'Período tranquilo'),
('pt-BR', 'inactive', 1, 'Pouca atividade recente'),
('pt-BR', 'inactive', 1, 'Ritmo leve'),
('pt-BR', 'inactive', 1, 'Modo descanso'),
('pt-BR', 'inactive', 2, 'Pouca atividade neste período.'),
('pt-BR', 'inactive', 2, 'As coisas esfriaram um pouco.'),
('pt-BR', 'inactive', 2, 'Quase nada aconteceu.'),
('pt-BR', 'inactive', 2, 'Pronto para retomar o ritmo?'),

-- pt-BR | focus
('pt-BR', 'focus', 1, 'Modo foco'),
('pt-BR', 'focus', 1, 'Em estado de fluxo'),
('pt-BR', 'focus', 1, 'Focado e tranquilo'),
('pt-BR', 'focus', 1, 'Trabalho profundo'),
('pt-BR', 'focus', 2, 'Sessões longas sem distração.'),
('pt-BR', 'focus', 2, 'Trabalho profundo traz resultado.'),
('pt-BR', 'focus', 2, 'Qualidade acima de quantidade.'),
('pt-BR', 'focus', 2, 'O foco está funcionando.'),

-- pt-BR | project
('pt-BR', 'project', 1, 'Motor do projeto'),
('pt-BR', 'project', 1, 'Construtor constante'),
('pt-BR', 'project', 1, 'Progresso visível'),
('pt-BR', 'project', 1, 'Projeto em andamento'),
('pt-BR', 'project', 2, 'O projeto está avançando.'),
('pt-BR', 'project', 2, 'Bom progresso na lista.'),
('pt-BR', 'project', 2, 'Passo a passo até o fim.'),
('pt-BR', 'project', 2, 'O final está mais próximo.'),

-- pt-BR | neutral
('pt-BR', 'neutral', 1, 'Ritmo constante'),
('pt-BR', 'neutral', 1, 'Sem novidades'),
('pt-BR', 'neutral', 1, 'Esforço contínuo'),
('pt-BR', 'neutral', 1, 'Período normal'),
('pt-BR', 'neutral', 2, 'Tudo segue seu curso.'),
('pt-BR', 'neutral', 2, 'Um período sem surpresas.'),
('pt-BR', 'neutral', 2, 'Trabalho estável e consistente.'),
('pt-BR', 'neutral', 2, 'Nada de especial a destacar.'),

-- ============================================================
-- tr-TR
-- ============================================================

-- tr-TR | success
('tr-TR', 'success', 1, 'Görev ezici'),
('tr-TR', 'success', 1, 'Son teslim avcısı'),
('tr-TR', 'success', 1, 'Tam gaz ileri'),
('tr-TR', 'success', 1, 'Listenin fatihi'),
('tr-TR', 'success', 2, 'Harika bir tempo yakaladın.'),
('tr-TR', 'success', 2, 'Liste hızla eriyor.'),
('tr-TR', 'success', 2, 'Böyle devam et.'),
('tr-TR', 'success', 2, 'Verimli bir dönem geçti.'),

-- tr-TR | perfect_day
('tr-TR', 'perfect_day', 1, 'Mükemmel gün'),
('tr-TR', 'perfect_day', 1, 'Temiz sayfa'),
('tr-TR', 'perfect_day', 1, 'Eksiksiz tamamlandı'),
('tr-TR', 'perfect_day', 1, 'Hiçbir şey kalmadı'),
('tr-TR', 'perfect_day', 2, 'Planlanan her şey yapıldı.'),
('tr-TR', 'perfect_day', 2, 'Tek bir görev kaçmadı.'),
('tr-TR', 'perfect_day', 2, 'İşte böyle yapılır.'),
('tr-TR', 'perfect_day', 2, 'Gün sıfırla kapandı.'),

-- tr-TR | overdue
('tr-TR', 'overdue', 1, 'Bazı görevler gecikti'),
('tr-TR', 'overdue', 1, 'Bekleyen işler var'),
('tr-TR', 'overdue', 1, 'Birikmiş görevler'),
('tr-TR', 'overdue', 1, 'Dikkat gereken maddeler'),
('tr-TR', 'overdue', 2, 'Listeye bakmaya değer.'),
('tr-TR', 'overdue', 2, 'Birkaç görev gecikmiş durumda.'),
('tr-TR', 'overdue', 2, 'Yetişme zamanı geldi.'),
('tr-TR', 'overdue', 2, 'Bazı işler süresi geçmiş.'),

-- tr-TR | inactive
('tr-TR', 'inactive', 1, 'Sakin bir dönem'),
('tr-TR', 'inactive', 1, 'Düşük aktivite'),
('tr-TR', 'inactive', 1, 'Yavaş mod'),
('tr-TR', 'inactive', 1, 'Çalışma duraklamış'),
('tr-TR', 'inactive', 2, 'Bu dönemde pek bir şey olmadı.'),
('tr-TR', 'inactive', 2, 'Aktivite çok düşük kaldı.'),
('tr-TR', 'inactive', 2, 'Her şey yavaşladı biraz.'),
('tr-TR', 'inactive', 2, 'Tekrar hız kazanmaya hazır mısın?'),

-- tr-TR | focus
('tr-TR', 'focus', 1, 'Derin çalışma modu'),
('tr-TR', 'focus', 1, 'Akış halinde'),
('tr-TR', 'focus', 1, 'Odaklı ve sakin'),
('tr-TR', 'focus', 1, 'Konsantrasyon ustası'),
('tr-TR', 'focus', 2, 'Uzun odak seansları tamamlandı.'),
('tr-TR', 'focus', 2, 'Derin çalışma meyvesini veriyor.'),
('tr-TR', 'focus', 2, 'Kalite miktardan önemli.'),
('tr-TR', 'focus', 2, 'Odaklanma işe yarıyor.'),

-- tr-TR | project
('tr-TR', 'project', 1, 'Proje motoru'),
('tr-TR', 'project', 1, 'Kararlı inşaatçı'),
('tr-TR', 'project', 1, 'İlerleme görünür'),
('tr-TR', 'project', 1, 'Proje yolunda'),
('tr-TR', 'project', 2, 'Proje ilerlemeye devam ediyor.'),
('tr-TR', 'project', 2, 'Listede iyi bir ilerleme var.'),
('tr-TR', 'project', 2, 'Adım adım hedefe doğru.'),
('tr-TR', 'project', 2, 'Bitiş çizgisi yaklaşıyor.'),

-- tr-TR | neutral
('tr-TR', 'neutral', 1, 'Sabit tempo'),
('tr-TR', 'neutral', 1, 'İşler yürüyor'),
('tr-TR', 'neutral', 1, 'Sürprizsiz bir dönem'),
('tr-TR', 'neutral', 1, 'Normal seyir'),
('tr-TR', 'neutral', 2, 'Her şey olağan seyrinde.'),
('tr-TR', 'neutral', 2, 'Tipik bir çalışma dönemi.'),
('tr-TR', 'neutral', 2, 'Özel bir şey yok.'),
('tr-TR', 'neutral', 2, 'Tutarlı çalışma sürüyor.'),

-- ============================================================
-- zh-CN
-- ============================================================

-- zh-CN | success
('zh-CN', 'success', 1, '任务终结者'),
('zh-CN', 'success', 1, '截止日狙击手'),
('zh-CN', 'success', 1, '高效达人'),
('zh-CN', 'success', 1, '清单粉碎机'),
('zh-CN', 'success', 2, '保持这个好势头。'),
('zh-CN', 'success', 2, '清单飞速缩短。'),
('zh-CN', 'success', 2, '就该这样干。'),
('zh-CN', 'success', 2, '这段时间很有成效。'),

-- zh-CN | perfect_day
('zh-CN', 'perfect_day', 1, '完美一天'),
('zh-CN', 'perfect_day', 1, '一项未落'),
('zh-CN', 'perfect_day', 1, '清盘收工'),
('zh-CN', 'perfect_day', 1, '全部按计划完成'),
('zh-CN', 'perfect_day', 2, '计划内的全部做完了。'),
('zh-CN', 'perfect_day', 2, '没有遗漏任何一项。'),
('zh-CN', 'perfect_day', 2, '就该这么做。'),
('zh-CN', 'perfect_day', 2, '今天干净收官。'),

-- zh-CN | overdue
('zh-CN', 'overdue', 1, '有任务逾期'),
('zh-CN', 'overdue', 1, '一些事项待处理'),
('zh-CN', 'overdue', 1, '积压在增加'),
('zh-CN', 'overdue', 1, '需要关注'),
('zh-CN', 'overdue', 2, '值得看一看清单。'),
('zh-CN', 'overdue', 2, '有几项任务已逾期。'),
('zh-CN', 'overdue', 2, '是时候追上进度了。'),
('zh-CN', 'overdue', 2, '部分任务已超时。'),

-- zh-CN | inactive
('zh-CN', 'inactive', 1, '平静的一段时间'),
('zh-CN', 'inactive', 1, '最近活动较少'),
('zh-CN', 'inactive', 1, '慢节奏模式'),
('zh-CN', 'inactive', 1, '工作暂缓'),
('zh-CN', 'inactive', 2, '这段时间几乎没有动静。'),
('zh-CN', 'inactive', 2, '活动量非常有限。'),
('zh-CN', 'inactive', 2, '事情慢了下来。'),
('zh-CN', 'inactive', 2, '准备好重新提速了吗？'),

-- zh-CN | focus
('zh-CN', 'focus', 1, '深度工作模式'),
('zh-CN', 'focus', 1, '进入心流'),
('zh-CN', 'focus', 1, '专注而平静'),
('zh-CN', 'focus', 1, '专注力达人'),
('zh-CN', 'focus', 2, '记录了大量专注时段。'),
('zh-CN', 'focus', 2, '深度工作正在发挥作用。'),
('zh-CN', 'focus', 2, '质量优于数量。'),
('zh-CN', 'focus', 2, '专注带来了成果。'),

-- zh-CN | project
('zh-CN', 'project', 1, '项目推进者'),
('zh-CN', 'project', 1, '稳步建设者'),
('zh-CN', 'project', 1, '进展有目共睹'),
('zh-CN', 'project', 1, '项目在推进中'),
('zh-CN', 'project', 2, '项目正在稳步推进。'),
('zh-CN', 'project', 2, '清单进展良好。'),
('zh-CN', 'project', 2, '一步一步接近目标。'),
('zh-CN', 'project', 2, '终点越来越近了。'),

-- zh-CN | neutral
('zh-CN', 'neutral', 1, '稳定前行'),
('zh-CN', 'neutral', 1, '工作在继续'),
('zh-CN', 'neutral', 1, '没什么特别的'),
('zh-CN', 'neutral', 1, '正常状态'),
('zh-CN', 'neutral', 2, '一切照常进行。'),
('zh-CN', 'neutral', 2, '平稳的一段时期。'),
('zh-CN', 'neutral', 2, '没有什么特别的。'),
('zh-CN', 'neutral', 2, '持续稳定的工作。'),

-- ============================================================
-- hi-IN
-- ============================================================

-- hi-IN | success
('hi-IN', 'success', 1, 'कार्य विजेता'),
('hi-IN', 'success', 1, 'समय-सीमा का शिकारी'),
('hi-IN', 'success', 1, 'उत्पादकता की मशीन'),
('hi-IN', 'success', 1, 'सूची का सफाया'),
('hi-IN', 'success', 2, 'यही रफ्तार बनाए रखो।'),
('hi-IN', 'success', 2, 'सूची तेज़ी से घट रही है।'),
('hi-IN', 'success', 2, 'बहुत उत्पादक समय रहा।'),
('hi-IN', 'success', 2, 'ऐसे ही करते हैं।'),

-- hi-IN | perfect_day
('hi-IN', 'perfect_day', 1, 'परफेक्ट दिन'),
('hi-IN', 'perfect_day', 1, 'कोई कमी नहीं'),
('hi-IN', 'perfect_day', 1, 'साफ स्लेट'),
('hi-IN', 'perfect_day', 1, 'सब पूरा हुआ'),
('hi-IN', 'perfect_day', 2, 'सभी योजनाएं पूरी हुईं।'),
('hi-IN', 'perfect_day', 2, 'एक भी काम छूटा नहीं।'),
('hi-IN', 'perfect_day', 2, 'यही तरीका है।'),
('hi-IN', 'perfect_day', 2, 'दिन शून्य पर बंद हुआ।'),

-- hi-IN | overdue
('hi-IN', 'overdue', 1, 'कुछ काम बाकी हैं'),
('hi-IN', 'overdue', 1, 'कुछ चीज़ें इंतज़ार में हैं'),
('hi-IN', 'overdue', 1, 'विलंब हो रहा है'),
('hi-IN', 'overdue', 1, 'ध्यान देने की जरूरत'),
('hi-IN', 'overdue', 2, 'सूची देखना उचित रहेगा।'),
('hi-IN', 'overdue', 2, 'कुछ कार्य देर से हैं।'),
('hi-IN', 'overdue', 2, 'अब पकड़ने का वक्त है।'),
('hi-IN', 'overdue', 2, 'कुछ काम समय से पीछे हैं।'),

-- hi-IN | inactive
('hi-IN', 'inactive', 1, 'शांत समय'),
('hi-IN', 'inactive', 1, 'कम गतिविधि'),
('hi-IN', 'inactive', 1, 'धीमी रफ्तार'),
('hi-IN', 'inactive', 1, 'काम ठहरा हुआ है'),
('hi-IN', 'inactive', 2, 'इस दौर में बहुत कम हुआ।'),
('hi-IN', 'inactive', 2, 'गतिविधि बेहद कम रही।'),
('hi-IN', 'inactive', 2, 'सब कुछ धीमा हो गया।'),
('hi-IN', 'inactive', 2, 'फिर से रफ्तार पकड़ने की बारी?'),

-- hi-IN | focus
('hi-IN', 'focus', 1, 'गहरे काम में'),
('hi-IN', 'focus', 1, 'प्रवाह में'),
('hi-IN', 'focus', 1, 'एकाग्र और शांत'),
('hi-IN', 'focus', 1, 'फोकस का माहिर'),
('hi-IN', 'focus', 2, 'लंबे फोकस सत्र दर्ज हुए।'),
('hi-IN', 'focus', 2, 'गहरा काम फल दे रहा है।'),
('hi-IN', 'focus', 2, 'गुणवत्ता मात्रा से बड़ी है।'),
('hi-IN', 'focus', 2, 'फोकस काम आ रहा है।'),

-- hi-IN | project
('hi-IN', 'project', 1, 'परियोजना का इंजन'),
('hi-IN', 'project', 1, 'स्थिर निर्माता'),
('hi-IN', 'project', 1, 'प्रगति स्पष्ट है'),
('hi-IN', 'project', 1, 'परियोजना आगे बढ़ रही है'),
('hi-IN', 'project', 2, 'परियोजना आगे बढ़ रही है।'),
('hi-IN', 'project', 2, 'सूची पर अच्छी प्रगति है।'),
('hi-IN', 'project', 2, 'कदम-दर-कदम लक्ष्य की ओर।'),
('hi-IN', 'project', 2, 'मंजिल करीब आ रही है।'),

-- hi-IN | neutral
('hi-IN', 'neutral', 1, 'स्थिर लय'),
('hi-IN', 'neutral', 1, 'काम चल रहा है'),
('hi-IN', 'neutral', 1, 'कोई खास बात नहीं'),
('hi-IN', 'neutral', 1, 'सामान्य स्थिति'),
('hi-IN', 'neutral', 2, 'सब अपनी रफ्तार से चल रहा है।'),
('hi-IN', 'neutral', 2, 'एक सामान्य कार्य अवधि।'),
('hi-IN', 'neutral', 2, 'कुछ खास नहीं।'),
('hi-IN', 'neutral', 2, 'नियमित काम जारी है।'),

-- ============================================================
-- bn-BD
-- ============================================================

-- bn-BD | success
('bn-BD', 'success', 1, 'কাজের মেশিন'),
('bn-BD', 'success', 1, 'সময়সীমার শিকারি'),
('bn-BD', 'success', 1, 'তালিকা ধ্বংসকারী'),
('bn-BD', 'success', 1, 'উৎপাদনশীলতার নায়ক'),
('bn-BD', 'success', 2, 'এই গতি বজায় রাখো।'),
('bn-BD', 'success', 2, 'তালিকা দ্রুত ছোট হচ্ছে।'),
('bn-BD', 'success', 2, 'চমৎকার একটি সময়কাল।'),
('bn-BD', 'success', 2, 'এভাবেই করতে হয়।'),

-- bn-BD | perfect_day
('bn-BD', 'perfect_day', 1, 'নিখুঁত দিন'),
('bn-BD', 'perfect_day', 1, 'কিছুই বাদ পড়েনি'),
('bn-BD', 'perfect_day', 1, 'পরিষ্কার স্লেট'),
('bn-BD', 'perfect_day', 1, 'সব সম্পন্ন'),
('bn-BD', 'perfect_day', 2, 'পরিকল্পিত সব কাজ শেষ।'),
('bn-BD', 'perfect_day', 2, 'একটিও কাজ মিস হয়নি।'),
('bn-BD', 'perfect_day', 2, 'এটাই সঠিক উপায়।'),
('bn-BD', 'perfect_day', 2, 'দিনটি শূন্যে শেষ হলো।'),

-- bn-BD | overdue
('bn-BD', 'overdue', 1, 'কিছু কাজ বাকি আছে'),
('bn-BD', 'overdue', 1, 'কিছু জিনিস অপেক্ষায়'),
('bn-BD', 'overdue', 1, 'বিলম্ব জমছে'),
('bn-BD', 'overdue', 1, 'মনোযোগ দরকার'),
('bn-BD', 'overdue', 2, 'তালিকাটি একবার দেখা উচিত।'),
('bn-BD', 'overdue', 2, 'কয়েকটি কাজ দেরিতে আছে।'),
('bn-BD', 'overdue', 2, 'এখন ধরে নেওয়ার সময়।'),
('bn-BD', 'overdue', 2, 'কিছু কাজ সময়ের বাইরে।'),

-- bn-BD | inactive
('bn-BD', 'inactive', 1, 'শান্ত সময়কাল'),
('bn-BD', 'inactive', 1, 'কম কার্যকলাপ'),
('bn-BD', 'inactive', 1, 'ধীর গতির মোড'),
('bn-BD', 'inactive', 1, 'কাজ থেমে আছে'),
('bn-BD', 'inactive', 2, 'এই সময়ে খুব কম হয়েছে।'),
('bn-BD', 'inactive', 2, 'কার্যকলাপ ছিল খুবই সীমিত।'),
('bn-BD', 'inactive', 2, 'সব কিছু একটু থেমেছে।'),
('bn-BD', 'inactive', 2, 'আবার গতিতে ফিরতে প্রস্তুত?'),

-- bn-BD | focus
('bn-BD', 'focus', 1, 'গভীর কাজের মোড'),
('bn-BD', 'focus', 1, 'প্রবাহে আছ'),
('bn-BD', 'focus', 1, 'মনোযোগী ও শান্ত'),
('bn-BD', 'focus', 1, 'মনোযোগের ওস্তাদ'),
('bn-BD', 'focus', 2, 'দীর্ঘ ফোকাস সেশন রেকর্ড হয়েছে।'),
('bn-BD', 'focus', 2, 'গভীর কাজ ফল দিচ্ছে।'),
('bn-BD', 'focus', 2, 'পরিমাণের চেয়ে মান গুরুত্বপূর্ণ।'),
('bn-BD', 'focus', 2, 'ফোকাস কাজ করছে।'),

-- bn-BD | project
('bn-BD', 'project', 1, 'প্রকল্পের চালক'),
('bn-BD', 'project', 1, 'স্থির নির্মাতা'),
('bn-BD', 'project', 1, 'অগ্রগতি স্পষ্ট'),
('bn-BD', 'project', 1, 'প্রকল্প এগিয়ে চলছে'),
('bn-BD', 'project', 2, 'প্রকল্প এগিয়ে যাচ্ছে।'),
('bn-BD', 'project', 2, 'তালিকায় ভালো অগ্রগতি।'),
('bn-BD', 'project', 2, 'ধাপে ধাপে লক্ষ্যের দিকে।'),
('bn-BD', 'project', 2, 'শেষ রেখা কাছে আসছে।'),

-- bn-BD | neutral
('bn-BD', 'neutral', 1, 'স্থিতিশীল ছন্দ'),
('bn-BD', 'neutral', 1, 'কাজ চলছে'),
('bn-BD', 'neutral', 1, 'কোনো চমক নেই'),
('bn-BD', 'neutral', 1, 'স্বাভাবিক অবস্থা'),
('bn-BD', 'neutral', 2, 'সব স্বাভাবিকভাবে চলছে।'),
('bn-BD', 'neutral', 2, 'একটি সাধারণ কাজের সময়কাল।'),
('bn-BD', 'neutral', 2, 'বিশেষ কিছু নেই।'),
('bn-BD', 'neutral', 2, 'ধারাবাহিক কাজ চলছে।'),

-- ============================================================
-- ja-JP
-- ============================================================

-- ja-JP | success
('ja-JP', 'success', 1, 'タスク粉砕機'),
('ja-JP', 'success', 1, '締め切りハンター'),
('ja-JP', 'success', 1, 'リスト制覇者'),
('ja-JP', 'success', 1, '絶好調'),
('ja-JP', 'success', 2, 'このペースを維持しよう。'),
('ja-JP', 'success', 2, 'リストがみるみる減っている。'),
('ja-JP', 'success', 2, '充実した期間だった。'),
('ja-JP', 'success', 2, 'これが正しいやり方だ。'),

-- ja-JP | perfect_day
('ja-JP', 'perfect_day', 1, '完璧な一日'),
('ja-JP', 'perfect_day', 1, '何も残さず'),
('ja-JP', 'perfect_day', 1, '全て完了'),
('ja-JP', 'perfect_day', 1, '計画通り'),
('ja-JP', 'perfect_day', 2, '予定した全てが終わった。'),
('ja-JP', 'perfect_day', 2, 'タスクを一つも逃さなかった。'),
('ja-JP', 'perfect_day', 2, 'これが理想の終わり方だ。'),
('ja-JP', 'perfect_day', 2, '今日はゼロで締めくくった。'),

-- ja-JP | overdue
('ja-JP', 'overdue', 1, '期限切れタスクあり'),
('ja-JP', 'overdue', 1, '待ち状態の項目'),
('ja-JP', 'overdue', 1, '積み残しが増えている'),
('ja-JP', 'overdue', 1, '確認が必要'),
('ja-JP', 'overdue', 2, 'リストを見直す価値がある。'),
('ja-JP', 'overdue', 2, 'いくつかのタスクが遅れている。'),
('ja-JP', 'overdue', 2, '追いつく時が来た。'),
('ja-JP', 'overdue', 2, '一部のタスクが期限を過ぎている。'),

-- ja-JP | inactive
('ja-JP', 'inactive', 1, '静かな期間'),
('ja-JP', 'inactive', 1, '最近の活動は少ない'),
('ja-JP', 'inactive', 1, 'ゆっくりモード'),
('ja-JP', 'inactive', 1, '作業が止まっている'),
('ja-JP', 'inactive', 2, 'この期間はほとんど動きがなかった。'),
('ja-JP', 'inactive', 2, '活動量がとても少なかった。'),
('ja-JP', 'inactive', 2, 'ペースが落ちている。'),
('ja-JP', 'inactive', 2, 'また動き出す準備はできている?'),

-- ja-JP | focus
('ja-JP', 'focus', 1, '深い作業モード'),
('ja-JP', 'focus', 1, 'フロー状態'),
('ja-JP', 'focus', 1, '集中と静けさ'),
('ja-JP', 'focus', 1, '集中力の達人'),
('ja-JP', 'focus', 2, '長い集中セッションを記録した。'),
('ja-JP', 'focus', 2, '深い作業が実を結んでいる。'),
('ja-JP', 'focus', 2, '量より質が大切だ。'),
('ja-JP', 'focus', 2, '集中力が機能している。'),

-- ja-JP | project
('ja-JP', 'project', 1, 'プロジェクトの推進役'),
('ja-JP', 'project', 1, '着実な構築者'),
('ja-JP', 'project', 1, '進捗が見える'),
('ja-JP', 'project', 1, 'プロジェクトが動いている'),
('ja-JP', 'project', 2, 'プロジェクトが前進している。'),
('ja-JP', 'project', 2, 'リストの進捗が良い。'),
('ja-JP', 'project', 2, '一歩一歩ゴールへ向かっている。'),
('ja-JP', 'project', 2, 'ゴールが近づいてきた。'),

-- ja-JP | neutral
('ja-JP', 'neutral', 1, '安定したペース'),
('ja-JP', 'neutral', 1, '作業は続いている'),
('ja-JP', 'neutral', 1, '特に変わりなし'),
('ja-JP', 'neutral', 1, 'いつも通り'),
('ja-JP', 'neutral', 2, '全てが順調に進んでいる。'),
('ja-JP', 'neutral', 2, '普通の作業期間だった。'),
('ja-JP', 'neutral', 2, '特に目立ったことはない。'),
('ja-JP', 'neutral', 2, '安定した作業が続いている。'),

-- ============================================================
-- ko-KR
-- ============================================================

-- ko-KR | success
('ko-KR', 'success', 1, '할 일 분쇄기'),
('ko-KR', 'success', 1, '마감 사냥꾼'),
('ko-KR', 'success', 1, '목록 정리의 달인'),
('ko-KR', 'success', 1, '풀가동 중'),
('ko-KR', 'success', 2, '이 속도 유지해 봐.'),
('ko-KR', 'success', 2, '목록이 빠르게 줄고 있어.'),
('ko-KR', 'success', 2, '생산적인 기간이었어.'),
('ko-KR', 'success', 2, '이게 바로 하는 방법이지.'),

-- ko-KR | perfect_day
('ko-KR', 'perfect_day', 1, '완벽한 하루'),
('ko-KR', 'perfect_day', 1, '남긴 것 없음'),
('ko-KR', 'perfect_day', 1, '깔끔하게 마무리'),
('ko-KR', 'perfect_day', 1, '계획대로 완수'),
('ko-KR', 'perfect_day', 2, '계획한 모든 것이 완료됐어.'),
('ko-KR', 'perfect_day', 2, '하나도 빠지지 않았어.'),
('ko-KR', 'perfect_day', 2, '바로 이렇게 하는 거야.'),
('ko-KR', 'perfect_day', 2, '오늘은 제로로 마감했어.'),

-- ko-KR | overdue
('ko-KR', 'overdue', 1, '밀린 할 일 있음'),
('ko-KR', 'overdue', 1, '기다리는 항목들'),
('ko-KR', 'overdue', 1, '쌓이는 미완료'),
('ko-KR', 'overdue', 1, '확인이 필요해'),
('ko-KR', 'overdue', 2, '목록을 한번 살펴볼 만해.'),
('ko-KR', 'overdue', 2, '몇 가지 할 일이 늦어졌어.'),
('ko-KR', 'overdue', 2, '따라잡을 시간이야.'),
('ko-KR', 'overdue', 2, '일부 항목이 기한을 넘겼어.'),

-- ko-KR | inactive
('ko-KR', 'inactive', 1, '조용한 시기'),
('ko-KR', 'inactive', 1, '최근 활동 적음'),
('ko-KR', 'inactive', 1, '느린 모드'),
('ko-KR', 'inactive', 1, '작업이 멈춰 있음'),
('ko-KR', 'inactive', 2, '이 기간에는 거의 없었어.'),
('ko-KR', 'inactive', 2, '활동량이 아주 적었어.'),
('ko-KR', 'inactive', 2, '속도가 많이 느려졌어.'),
('ko-KR', 'inactive', 2, '다시 속도 낼 준비됐어?'),

-- ko-KR | focus
('ko-KR', 'focus', 1, '집중 작업 모드'),
('ko-KR', 'focus', 1, '몰입 상태'),
('ko-KR', 'focus', 1, '집중하고 차분해'),
('ko-KR', 'focus', 1, '집중력 고수'),
('ko-KR', 'focus', 2, '긴 집중 세션을 기록했어.'),
('ko-KR', 'focus', 2, '깊은 작업이 효과를 내고 있어.'),
('ko-KR', 'focus', 2, '양보다 질이 중요해.'),
('ko-KR', 'focus', 2, '집중력이 효과를 발휘하고 있어.'),

-- ko-KR | project
('ko-KR', 'project', 1, '프로젝트 추진자'),
('ko-KR', 'project', 1, '꾸준한 건설자'),
('ko-KR', 'project', 1, '진척이 보여'),
('ko-KR', 'project', 1, '프로젝트 진행 중'),
('ko-KR', 'project', 2, '프로젝트가 앞으로 나아가고 있어.'),
('ko-KR', 'project', 2, '목록에서 좋은 진전을 보이고 있어.'),
('ko-KR', 'project', 2, '한 걸음씩 목표로 가고 있어.'),
('ko-KR', 'project', 2, '결승선이 가까워지고 있어.'),

-- ko-KR | neutral
('ko-KR', 'neutral', 1, '꾸준한 페이스'),
('ko-KR', 'neutral', 1, '작업은 계속돼'),
('ko-KR', 'neutral', 1, '특별한 건 없어'),
('ko-KR', 'neutral', 1, '평소와 같은 상태'),
('ko-KR', 'neutral', 2, '모든 게 순조롭게 진행 중이야.'),
('ko-KR', 'neutral', 2, '평범한 작업 기간이었어.'),
('ko-KR', 'neutral', 2, '특별한 건 없었어.'),
('ko-KR', 'neutral', 2, '꾸준한 작업이 계속되고 있어.')

;
