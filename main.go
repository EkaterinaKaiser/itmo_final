package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/lib/pq"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "validator"
	password = "val1dat0r"
	dbname   = "project-sem-1"
)

var db *sql.DB

func initDatabase() error {
	dropTableQuery := `DROP TABLE IF EXISTS prices;`
	_, err := db.Exec(dropTableQuery)
	if err != nil {
		return fmt.Errorf("ошибка удаления таблицы: %v", err)
	}

	createTableQuery := `
    CREATE TABLE prices (
        id SERIAL PRIMARY KEY,
        name VARCHAR(255) NOT NULL,
        category VARCHAR(255) NOT NULL,
        price NUMERIC(10, 2) NOT NULL,
        create_date TIMESTAMP NOT NULL
    );`

	_, err = db.Exec(createTableQuery)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы: %v", err)
	}

	log.Println("Таблица 'prices' успешно пересоздана")
	return nil
}

func setupRoutes() *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("/api/v0/prices", handlePrices)
	return router
}

func handlePrices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handlePostPrices(w, r)
	case http.MethodGet:
		handleGetPrices(w, r)
	default:
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}
}

func connectDB() error {
	var err error
	db, err = sql.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname))
	if err != nil {
		return fmt.Errorf("ошибка подключения к базе данных: %v", err)
	}
	return nil
}

func main() {
	// Инициализация базы данных
	if err := connectDB(); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Создание таблицы
	if err := initDatabase(); err != nil {
		log.Fatal(err)
	}

	// Настройка маршрутизации
	router := setupRoutes()

	// Запуск сервера
	fmt.Println("Server started on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}
}

func handlePostPrices(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error reading file", http.StatusBadRequest)
		log.Printf("Ошибка чтения файла: %v", err)
		return
	}
	defer file.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(file)
	if err != nil {
		http.Error(w, "Error reading file content", http.StatusBadRequest)
		log.Printf("Ошибка чтения содержимого файла: %v", err)
		return
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		http.Error(w, "Error unzipping file", http.StatusBadRequest)
		log.Printf("Ошибка распаковки ZIP-архива: %v", err)
		return
	}

	var records [][]string
	var totalItems int
	var totalPrice float64
	categorySet := make(map[string]struct{})

	// Чтение и проверка данных из CSV
	for _, f := range reader.File {
		if f.Name == "data.csv" {
			csvFile, err := f.Open()
			if err != nil {
				http.Error(w, "Error opening CSV file", http.StatusInternalServerError)
				log.Printf("Ошибка открытия CSV-файла: %v", err)
				return
			}
			defer csvFile.Close()

			rows, err := csv.NewReader(csvFile).ReadAll()
			if err != nil {
				http.Error(w, "Error reading CSV", http.StatusInternalServerError)
				log.Printf("Ошибка чтения CSV-файла: %v", err)
				return
			}

			// Проверка данных, начиная со второй строки
			for _, row := range rows[1:] { // Пропускаем заголовок
				if len(row) < 5 { // Убедимся, что в строке достаточно столбцов
					log.Printf("Пропущена строка: недостаточно данных")
					continue
				}

				// Пропускаем первый столбец (id) и читаем остальные данные
				name := row[1]       // Второй столбец
				category := row[2]   // Третий столбец
				priceStr := row[3]   // Четвертый столбец
				createDate := row[4] // Пятый столбец

				// Проверка на пустые значения
				if name == "" || category == "" || priceStr == "" || createDate == "" {
					log.Printf("Пропущена строка: пустые значения")
					continue
				}

				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					log.Printf("Ошибка преобразования цены: %v, строка: %v", err, row)
					continue
				}

				createDateParsed, err := time.Parse("2006-01-02", createDate)
				if err != nil {
					log.Printf("Ошибка парсинга даты: %v, строка: %v", err, row)
					continue
				}

				// Добавляем только нужные данные в правильном порядке
				records = append(records, []string{
					name,
					category,
					fmt.Sprintf("%.2f", price),
					createDateParsed.Format("2006-01-02 15:04:05"),
				})

				totalItems++
				totalPrice += price
				categorySet[category] = struct{}{}
			}
		}
	}

	// Открытие транзакции и вставка данных
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Transaction error", http.StatusInternalServerError)
		log.Printf("Ошибка начала транзакции: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
        INSERT INTO prices (name, category, price, create_date) 
        VALUES ($1, $2, $3, $4::timestamp)
    `)
	if err != nil {
		http.Error(w, "SQL preparation error", http.StatusInternalServerError)
		log.Printf("Ошибка подготовки SQL-запроса: %v", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	for _, record := range records {
		_, err = stmt.Exec(record[0], record[1], record[2], record[3])
		if err != nil {
			log.Printf("Ошибка выполнения запроса: %v, строка: %v", err, record)
			tx.Rollback()
			http.Error(w, "Error inserting data", http.StatusInternalServerError)
			return
		}
	}

	var totalCategories int
	err = tx.QueryRow("SELECT COUNT(DISTINCT category) FROM prices").Scan(&totalCategories)
	if err != nil {
		log.Printf("Ошибка подсчета total_categories: %v", err)
		tx.Rollback()
		http.Error(w, "Error calculating total categories", http.StatusInternalServerError)
		return
	}

	err = tx.Commit()
	if err != nil {
		log.Printf("Ошибка завершения транзакции: %v", err)
		http.Error(w, "Transaction commit error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"total_items":      totalItems,
		"total_categories": totalCategories,
		"total_price":      fmt.Sprintf("%.2f", totalPrice),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleGetPrices(w http.ResponseWriter, _ *http.Request) {
	rows, err := db.Query("SELECT id, name, category, price, create_date FROM prices")
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		log.Printf("Ошибка запроса данных из базы: %v", err)
		return
	}
	defer rows.Close()

	var records [][]string
	// Добавляем заголовки в том же формате, что и во входном файле
	records = append(records, []string{"id", "name", "category", "price", "create_date"})

	for rows.Next() {
		var id int
		var name, category string
		var price float64
		var createDate time.Time

		err := rows.Scan(&id, &name, &category, &price, &createDate)
		if err != nil {
			http.Error(w, "Error scanning rows", http.StatusInternalServerError)
			log.Printf("Ошибка чтения строки из базы: %v", err)
			return
		}

		records = append(records, []string{
			strconv.Itoa(id),
			name,
			category,
			fmt.Sprintf("%.2f", price),
			createDate.Format("2006-01-02"), // Изменяем формат даты на YYYY-MM-DD
		})
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error during rows iteration", http.StatusInternalServerError)
		log.Printf("Ошибка во время итерации по строкам: %v", err)
		return
	}

	csvData := &bytes.Buffer{}
	writer := csv.NewWriter(csvData)
	for _, record := range records {
		if err := writer.Write(record); err != nil {
			http.Error(w, "Error writing CSV", http.StatusInternalServerError)
			log.Printf("Ошибка записи в CSV: %v", err)
			return
		}
	}
	writer.Flush()

	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)
	fileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		http.Error(w, "Error creating file in ZIP archive", http.StatusInternalServerError)
		log.Printf("Ошибка создания файла в ZIP-архиве: %v", err)
		return
	}

	if _, err := io.Copy(fileWriter, csvData); err != nil {
		http.Error(w, "Error copying data to ZIP archive", http.StatusInternalServerError)
		log.Printf("Ошибка записи данных в ZIP-архив: %v", err)
		return
	}

	if err := zipWriter.Close(); err != nil {
		http.Error(w, "Error closing ZIP archive", http.StatusInternalServerError)
		log.Printf("Ошибка закрытия ZIP-архива: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=response.zip")
	w.Write(zipBuffer.Bytes())
}
