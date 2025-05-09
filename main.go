package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	mongoURI     string
	databaseName string
	serverPort   string
)

func main() {
	log.Println("Запуск MCP сервера для MongoDB...")

	// Загрузка конфигурации из переменных окружения
	mongoURI = os.Getenv("MCPMONGO_MONGO_URI")
	if mongoURI == "" {
		log.Fatal("Переменная окружения MCPMONGO_MONGO_URI не установлена.")
	}

	databaseName = os.Getenv("MCPMONGO_DATABASE_NAME")
	if databaseName == "" {
		log.Fatal("Переменная окружения MCPMONGO_DATABASE_NAME не установлена.")
	}

	serverPort = os.Getenv("MCPMONGO_SERVER_PORT")
	if serverPort == "" {
		serverPort = "26275" // Порт по умолчанию, если не указан
		log.Printf("Переменная окружения MCPMONGO_SERVER_PORT не установлена. Используется порт по умолчанию: %s", serverPort)
	}

	// Подключение к MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Ошибка подключения к MongoDB: %v", err)
	}
	defer func() {
		if err = client.Disconnect(context.Background()); err != nil {
			log.Fatalf("Ошибка отключения от MongoDB: %v", err)
		}
	}()

	// Проверка подключения
	ctxPing, cancelPing := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelPing()
	err = client.Ping(ctxPing, readpref.Primary())
	if err != nil {
		log.Fatalf("Не удалось подключиться к MongoDB: %v", err)
	}
	log.Println("Успешно подключено к MongoDB!")

	db := client.Database(databaseName)

	// Создание нового MCP сервера
	s := server.NewMCPServer(
		"MongoDB MCP Server",
		"0.1.0",
	)

	// Определение инструмента для выполнения команд MongoDB
	executeMongoCommandTool := mcp.NewTool(
		"execute_mongo_command",
		mcp.WithDescription("Выполняет команду MongoDB (в формате JSON) в указанной базе данных. Возвращает результат в виде JSON."),
		mcp.WithString("command",
			mcp.Description("Команда MongoDB в формате JSON (например, {\"ping\": 1} или {\"find\": \"имя_коллекции\", \"filter\": {\"поле\": \"значение\"}})."),
			mcp.Required(),
		),
	)

	s.AddTool(executeMongoCommandTool, func(toolCtx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Printf("Получен запрос на вызов инструмента: Method=%s, Params=%+v", request.Method, request.Params)

		commandStr, ok := request.Params.Arguments["command"].(string)
		if !ok {
			return mcp.NewToolResultError("Параметр 'command' должен быть строкой."), nil
		}

		var commandDoc bson.D
		err := bson.UnmarshalExtJSON([]byte(commandStr), true, &commandDoc)
		if err != nil {
			log.Printf("Ошибка разбора JSON команды: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Ошибка разбора JSON команды: %v", err)), nil
		}

		log.Printf("Выполнение команды MongoDB: %v в базе данных '%s'", commandDoc, databaseName)

		var result bson.M
		// Используем ordered документ для мульти-ключевых команд
		err = db.RunCommand(toolCtx, commandDoc).Decode(&result)
		if err != nil {
			log.Printf("Ошибка выполнения команды MongoDB: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Ошибка выполнения команды MongoDB: %v", err)), nil
		}

		resultJSON, err := bson.MarshalExtJSON(result, true, true)
		if err != nil {
			log.Printf("Ошибка преобразования результата в JSON: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Ошибка преобразования результата в JSON: %v", err)), nil
		}

		responseText := string(resultJSON)
		log.Printf("Результат выполнения команды: %s", responseText)
		return mcp.NewToolResultText(responseText), nil
	})
	log.Println("Инструмент 'execute_mongo_command' добавлен.")

	// Запуск сервера
	log.Printf("MCP сервер для MongoDB готов к подключению через Stdio.")
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Ошибка запуска MCP сервера: %v", err)
	}
}
