-- Категории
INSERT INTO categories (name, description) VALUES
('Процессор',     'CPU — центральные процессоры'),
('Видеокарта',    'GPU — дискретные и интегрированные видеокарты'),
('Материнская плата', 'Motherboard'),
('Оперативная память', 'RAM — модули DDR4/DDR5'),
('Накопитель',    'SSD и HDD');

-- Производители
INSERT INTO manufacturers (name, country, founded_year) VALUES
('Intel',      'США', 1968),
('AMD',        'США', 1969),
('NVIDIA',     'США', 1993),
('ASUS',       'Тайвань', 1989),
('Samsung',    'Южная Корея', 1969),
('Kingston',   '  США', 1987);

-- Компоненты (без specs, price — целые числа)
INSERT INTO components (name, category_id, manufacturer_id, model, price) VALUES
('Intel Core i9-13900K', 1, 1, 'i9-13900K', 590),
('AMD Ryzen 9 7950X',    1, 2, '7950X',     650),
('NVIDIA RTX 4090',      2, 3, 'Founders Edition', 1600),
('ASUS ROG Strix X670E', 3, 4, 'X670E-E Gaming', 500),
('Samsung 990 PRO 2TB',  5, 5, 'MZ-V9P2T0BW', 180),
('Kingston Fury 32GB Kit', 4, 6, 'KF560C40-32', 130);

-- Остатки (без last_updated)
INSERT INTO stock (component_id, quantity, warehouse_location) VALUES
((SELECT id FROM components WHERE model = 'i9-13900K'),      23, 'Склад-A'),
((SELECT id FROM components WHERE model = '7950X'),          15, 'Склад-A'),
((SELECT id FROM components WHERE model = 'Founders Edition'),40, 'Склад-B'),
((SELECT id FROM components WHERE model = 'X670E-E Gaming'), 8, 'Склад-A'),
((SELECT id FROM components WHERE model = 'MZ-V9P2T0BW'),    112, 'Склад-C'),
((SELECT id FROM components WHERE model = 'KF560C40-32'),    67, 'Склад-B');
