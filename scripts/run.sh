#!/bin/bash

# Запуск приложения
go run main.go &
SERVER_PID=$! # Сохраняем PID сервера

# Ждём, пока сервер начнёт работу
# sleep 2

# Выводим сообщение об успешном запуске
echo "Server started in background with PID $SERVER_PID"