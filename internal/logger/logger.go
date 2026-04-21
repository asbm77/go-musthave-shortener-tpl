package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.SugaredLogger

func Initialize(level string) error {
	var config zap.Config

	if level == "debug" {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	logger, err := config.Build()
	if err != nil {
		return err
	}

	Logger = logger.Sugar()
	return nil
}

func init() {
	// Настройка энкодера для вывода в формате JSON (удобно для парсинга)
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder // Стандартный формат времени

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(config),           // Энкодер
		zapcore.Lock(zapcore.AddSync(os.Stdout)), // Вывод в stdout
		zap.NewAtomicLevelAt(zap.InfoLevel),      // Уровень логирования INFO
	)

	// Создаем логгер без захвата стека вызовов (для производительности)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
	Logger = zapLogger.Sugar() // Оборачиваем в SugaredLogger
}
