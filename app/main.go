// main.go
package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	SSLMode  string `yaml:"sslmode"`
}

var allowedTables = map[string]bool{
	"categories":     true,
	"manufacturers":  true,
	"components":     true,
	"stock":          true,
}

var tableColumns = map[string][]string{
	"categories":     {"id", "name"},
	"manufacturers":  {"id", "name", "country"},
	"components":     {"id", "category_id", "manufacturer_id", "model", "price", "release_year"},
	"stock":          {"id", "component_id", "warehouse", "quantity", "last_updated"},
}

var logFile *os.File

func initLog() {
	logPath := os.Getenv("LOG_FILE")
	if logPath != "" {
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal("Не удалось открыть лог-файл:", err)
		}
		log.SetOutput(logFile)
	}
}

func friendlyError(err error) string {
	if strings.Contains(err.Error(), "authentication failed") {
		return "Неверный логин или пароль для базы данных"
	}
	if strings.Contains(err.Error(), "connection refused") {
		return "Не удалось подключиться к серверу базы данных"
	}
	if strings.Contains(err.Error(), "relation") && strings.Contains(err.Error(), "does not exist") {
		return "Запрашиваемая таблица не существует"
	}
	if strings.Contains(err.Error(), "permission denied") {
		return "У вас нет прав на выполнение этой операции"
	}
	return "Произошла ошибка при работе с базой данных"
}

func main() {
	_ = godotenv.Load() // загружаем .env если есть
	initLog()
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	data, _ := os.ReadFile("config.yaml")
	var cfg Config
	_ = yaml.Unmarshal(data, &cfg)

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Логин БД: ")
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)
	fmt.Print("Пароль БД: ")
	passBytes, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	pass := strings.TrimSpace(string(passBytes))

	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		os.Getenv("DB_HOST"), 5432, os.Getenv("DB_NAME"), user, pass, cfg.SSLMode)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, friendlyError(err))
		log.Println("CONNECT ERROR:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)
	fmt.Println("Подключение к БД успешно")
	log.Println("Подключение успешно")

	db := sql.OpenDB(pgx.NewConnector(pgx.ConnectConfig{Config: conn.Config}))

	for {
		fmt.Println("\n=== Меню ===")
		fmt.Println("1. Просмотр таблиц")
		fmt.Println("2. Обновление записей")
		fmt.Println("3. Добавление записей")
		fmt.Println("4. Выход")
		fmt.Print("Выбор: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			viewMenu(ctx, db, reader)
		case "2":
			updateMenu(ctx, db, reader)
		case "3":
			insertMenu(ctx, db, reader)
		case "4":
			fmt.Println("До свидания!")
			return
		default:
			fmt.Println("Неверный выбор")
		}
	}
}

// ======================== ПРОСМОТР ========================
func viewMenu(ctx context.Context, db *sql.DB, r *bufio.Reader) {
	fmt.Print("Таблица (categories/manufacturers/components/stock): ")
	table, _ := r.ReadString('\n')
	table = strings.TrimSpace(table)
	if !allowedTables[table] {
		fmt.Println("Таблица не разрешена")
		return
	}

	fmt.Println("1. Все записи\n2. Фильтр по одному полю\n3. Фильтр по нескольким полям")
	fmt.Print("Выбор: ")
	mode, _ := r.ReadString('\n')
	mode = strings.TrimSpace(mode)

	var query string
	var args []interface{}

	switch mode {
	case "1":
		query = fmt.Sprintf("SELECT * FROM %s", table)
	case "2":
		col := askColumn(r, table)
		val := askValue(r, col)
		query = fmt.Sprintf("SELECT * FROM %s WHERE %s = $1", table, col)
		args = []interface{}{val}
	case "3":
		conditions := []string{}
		for {
			col := askColumn(r, table)
			val := askValue(r, col)
			conditions = append(conditions, fmt.Sprintf("%s = $%d", col, len(conditions)+1))
			args = append(args, val)
			fmt.Print("Добавить ещё условие? (y/n): ")
			more, _ := r.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(more)) != "y" {
				break
			}
		}
		query = fmt.Sprintf("SELECT * FROM %s WHERE %s", table, strings.Join(conditions, " AND "))
	default:
		fmt.Println("Неверный режим")
		return
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, friendlyError(err))
		log.Println("VIEW ERROR:", err, query, args)
		return
	}
	defer rows.Close()

	printRows(rows)
}

// ======================== ОБНОВЛЕНИЕ ========================
func updateMenu(ctx context.Context, db *sql.DB, r *bufio.Reader) {
	fmt.Print("Таблица: ")
	table, _ := r.ReadString('\n')
	table = strings.TrimSpace(table)
	if !allowedTables[table] {
		fmt.Println("Таблица не разрешена")
		return
	}

	fmt.Println("1. Одна запись (по id)\n2. Несколько записей (по списку значений)")
	fmt.Print("Выбор: ")
	mode, _ := r.ReadString('\n')
	mode = strings.TrimSpace(mode)

	if mode == "1" {
		id := askInt(r, "ID записи")
		updates := map[string]interface{}{}
		for {
			col := askColumn(r, table)
			if col == "id" {
				fmt.Println("Поле id нельзя менять")
				continue
			}
			val := askValue(r, col)
			updates[col] = val
			fmt.Print("Ещё поле? (y/n): ")
			more, _ := r.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(more)) != "y" {
				break
			}
		}
		setParts := []string{}
		args := []interface{}{}
		i := 1
		for col, val := range updates {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, i))
			args = append(args, val)
			i++
		}
		args = append(args, id)
		query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", table, strings.Join(setParts, ", "), i)
		res, err := db.ExecContext(ctx, query, args...)
		if err != nil {
			fmt.Fprintln(os.Stderr, friendlyError(err))
			log.Println("UPDATE ERROR:", err, query)
			return
		}
		rows, _ := res.RowsAffected()
		fmt.Printf("Обновлено строк: %d\n", rows)
		log.Printf("UPDATE SUCCESS: %d rows", rows)
	} else if mode == "2" {
		col := askColumn(r, table)
		newVal := askValue(r, col)
		list := askList(r, col)
		placeholders := make([]string, len(list))
		args := make([]interface{}, len(list)+2)
		args[0] = newVal
		for i := range list {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args[i+1] = list[i]
		}
		query := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s IN (%s)", table, col, col, strings.Join(placeholders, ","))
		res, err := db.ExecContext(ctx, query, args...)
		if err != nil {
			fmt.Fprintln(os.Stderr, friendlyError(err))
			log.Println("BATCH UPDATE ERROR:", err)
			return
		}
		rows, _ := res.RowsAffected()
		fmt.Printf("Обновлено строк: %d\n", rows)
	}
}

// ======================== ВСТАВКА ========================
func insertMenu(ctx context.Context, db *sql.DB, r *bufio.Reader) {
	fmt.Println("1. Одна строка в одну таблицу")
	fmt.Println("2. Одна строка → несколько таблиц (components + stock)")
	fmt.Println("3. Несколько строк в одну таблицу")
	fmt.Print("Выбор: ")
	choice, _ := r.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1", "3":
		fmt.Print("Таблица: ")
		table, _ := r.ReadString('\n')
		table = strings.TrimSpace(table)
		if !allowedTables[table] {
			fmt.Println("Таблица не разрешена")
			return
		}
		cols := tableColumns[table]
		values := make([]interface{}, len(cols)-1) // без id
		for i := 1; i < len(cols); i++ {
			values[i-1] = askValue(r, cols[i])
		}
		placeholders := make([]string, len(values))
		for i := range placeholders {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(cols[1:], ", "), strings.Join(placeholders, ", "))
		if choice == "3" {
			query += " RETURNING id"
		}
		if choice == "1" {
			_, err := db.ExecContext(ctx, query, values...)
			if err != nil {
				fmt.Fprintln(os.Stderr, friendlyError(err))
				log.Println("INSERT ERROR:", err)
				return
			}
			fmt.Println("Строка добавлена")
		} else {
			// множественная вставка
			fmt.Print("Сколько строк добавить? ")
			nStr, _ := r.ReadString('\n')
			n, _ := strconv.Atoi(strings.TrimSpace(nStr))
			tx, _ := db.BeginTx(ctx, nil)
			for i := 0; i < n; i++ {
				fmt.Printf("--- Строка %d ---\n", i+1)
				for j := 1; j < len(cols); j++ {
					values[j-1] = askValue(r, cols[j])
				}
				_, err := tx.ExecContext(ctx, query, values...)
				if err != nil {
					tx.Rollback()
					fmt.Fprintln(os.Stderr, friendlyError(err))
					return
				}
			}
			tx.Commit()
			fmt.Printf("Добавлено %d строк\n", n)
		}
	case "2":
		// components → stock
		tx, _ := db.BeginTx(ctx, nil)
		// вставляем в components
		compCols := []string{"category_id", "manufacturer_id", "model", "price", "release_year"}
		compVals := make([]interface{}, len(compCols))
		for i, c := range compCols {
			compVals[i] = askValue(r, c)
		}
		compQuery := `INSERT INTO components (category_id, manufacturer_id, model, price, release_year) 
                      VALUES ($1, $2, $3, $4, $5) RETURNING id`
		var compID int
		err := tx.QueryRowContext(ctx, compQuery, compVals...).Scan(&compID)
		if err != nil {
			tx.Rollback()
			fmt.Fprintln(os.Stderr, friendlyError(err))
			return
		}
		// вставляем в stock
		stockQuery := `INSERT INTO stock (component_id, warehouse, quantity) VALUES ($1, $2, $3)`
		warehouse := askValue(r, "warehouse")
		quantity := askInt(r, "quantity")
		_, err = tx.ExecContext(ctx, stockQuery, compID, warehouse, quantity)
		if err != nil {
			tx.Rollback()
			fmt.Fprintln(os.Stderr, friendlyError(err))
			return
		}
		tx.Commit()
		fmt.Printf("Добавлен компонент с ID=%d и запись на склад\n", compID)
	}
}

// ======================== ВСПОМОГАТЕЛЬНЫЕ ========================
func askColumn(r *bufio.Reader, table string) string {
	fmt.Printf("Доступные колонки: %v\nКолонка: ", tableColumns[table])
	col, _ := r.ReadString('\n')
	col = strings.TrimSpace(col)
	for _, c := range tableColumns[table] {
		if c == col {
			return col
		}
	}
	fmt.Println("Недопустимая колонка, использую id")
	return "id"
}

func askValue(r *bufio.Reader, col string) interface{} {
	fmt.Print(col + " = ")
	val, _ := r.ReadString('\n')
	val = strings.TrimSpace(val)
	if strings.Contains(col, "price") {
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	if strings.HasSuffix(col, "_id") || col == "quantity" || col == "release_year" {
		i, _ := strconv.Atoi(val)
		return i
	}
	return val
}

func askInt(r *bufio.Reader, prompt string) int {
	for {
		fmt.Print(prompt + ": ")
		s, _ := r.ReadString('\n')
		i, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil {
			return i
		}
		fmt.Println("Введите число")
	}
}

func askList(r *bufio.Reader, col string) []interface{} {
	fmt.Printf("Значения %s через запятую: ", col)
	line, _ := r.ReadString('\n')
	parts := strings.Split(strings.TrimSpace(line), ",")
	res := make([]interface{}, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if strings.Contains(col, "price") {
			f, _ := strconv.ParseFloat(p, 64)
			res[i] = f
		} else if strings.HasSuffix(col, "_id") || col == "quantity" {
			iv, _ := strconv.Atoi(p)
			res[i] = iv
		} else {
			res[i] = p
		}
	}
	return res
}

func printRows(rows *sql.Rows) {
	cols, _ := rows.Columns()
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}
	for rows.Next() {
		rows.Scan(valuePtrs...)
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				fmt.Printf("%s: %s  ", col, string(b))
			} else {
				fmt.Printf("%s: %v  ", col, val)
			}
		}
		fmt.Println()
	}
}
