-- 1. Категории
CREATE TABLE categories (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) UNIQUE NOT NULL,
    description TEXT
);

-- 2. Производители
CREATE TABLE manufacturers (
    id            SERIAL PRIMARY KEY,
    name          VARCHAR(100) UNIQUE NOT NULL,
    country       VARCHAR(100),
    founded_year  INTEGER CHECK (founded_year > 1900 AND founded_year <= EXTRACT(YEAR FROM CURRENT_DATE))
);

-- 3. Компоненты (specs удалён, price теперь INTEGER)
CREATE TABLE components (
    id              SERIAL PRIMARY KEY,
    name            VARCHAR(200) NOT NULL,
    category_id     INTEGER NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    manufacturer_id INTEGER NOT NULL REFERENCES manufacturers(id) ON DELETE RESTRICT,
    model           VARCHAR(150),
    price           INTEGER CHECK (price >= 0)
);

-- 4. Остатки (last_updated удалён)
CREATE TABLE stock (
    id                  SERIAL PRIMARY KEY,
    component_id        INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    quantity            INTEGER NOT NULL CHECK (quantity >= 0),
    warehouse_location  VARCHAR(100)
);

-- Индексы (по желанию)
CREATE INDEX idx_components_category ON components(category_id);
CREATE INDEX idx_components_manufacturer ON components(manufacturer_id);
CREATE INDEX idx_stock_component ON stock(component_id);
