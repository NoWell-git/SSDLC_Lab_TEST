-- Создаём БД и пользователя
CREATE DATABASE pc_components;
\c pc_components

-- Таблицы
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    description TEXT
);

CREATE TABLE manufacturers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    country VARCHAR(50)
);

CREATE TABLE components (
    id SERIAL PRIMARY KEY,
    name VARCHAR(150) NOT NULL,
    category_id INT REFERENCES categories(id) ON DELETE CASCADE,
    manufacturer_id INT REFERENCES manufacturers(id),
    model VARCHAR(100),
    price DECIMAL(10,2) CHECK (price >= 0),
    release_year INT,
    in_stock BOOLEAN DEFAULT TRUE
);

CREATE TABLE compatibility (
    cpu_id INT REFERENCES components(id) ON DELETE CASCADE,
    motherboard_id INT REFERENCES components(id) ON DELETE CASCADE,
    socket VARCHAR(20),
    PRIMARY KEY (cpu_id, motherboard_id)
);

-- Пользователь с ограниченными правами
CREATE USER guest WITH PASSWORD '123';
GRANT CONNECT ON DATABASE pc_components TO guest;
GRANT USAGE ON SCHEMA public TO guest;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO guest;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO guest;

-- Тестовые данные
INSERT INTO categories (name) VALUES ('CPU'), ('Motherboard'), ('RAM'), ('GPU');
INSERT INTO manufacturers (name, country) VALUES ('Intel', 'USA'), ('AMD', 'USA'), ('ASUS', 'Taiwan');
INSERT INTO components (name, category_id, manufacturer_id, model, price, release_year) VALUES
('i9-13900K', 1, 1, '13900K', 589.99, 2022),
('Ryzen 9 7950X', 1, 2, '7950X', 699.00, 2022),
('ROG Z790-E', 2, 3, 'Z790-E', 479.99, 2023);
