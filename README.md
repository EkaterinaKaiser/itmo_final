# Финальный проект 1 семестра

REST API сервис для загрузки и выгрузки данных о ценах.

Локально post и get запросы работают полностью корректно. Но автотесты почему-то не проходятся.

## Требования к системе

Операционная система Ubnutu 24.04.
Требуется установленная PostgreSQL.
Порты 5432 и 8080 должны быть свободны.
Go = 1.23.3

## Установка и запуск

Необходимо настроить postgresql для корректной работы приложения:

sudo -u postgres psql -c 'CREATE DATABASE "project-sem-1";'
sudo -u postgres psql -c "CREATE USER validator WITH PASSWORD 'val1dat0r';"
sudo -u postgres psql -c 'GRANT ALL PRIVILEGES ON DATABASE "project-sem-1" TO validator;'
sudo -u postgres psql -d project-sem-1 -c "GRANT CREATE ON SCHEMA public TO validator;"

Запуск приложения из его директории с помощью команды go run main.go

## Тестирование

Тестирование можно провести путем отправки post и get запросов, например:
curl -X POST -F "file=@путь_до_файла/data.zip" http://localhost:8080/api/v0/prices
файл находится в архиве sample_data.zip в директории проекта


## Контакт

t.me/katerina_kaiser
