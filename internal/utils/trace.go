package utils

import (
	"fmt"
	"go-webapi/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TraceConfigDetails(logger *zap.Logger, cfg *config.Config) {
	if logger == nil || cfg == nil {
		fmt.Println("[WARN] logger or config is nil in logLoadedConfigDetails")
		return
	}
	maskedOracleConn := MaskOracleConnString(cfg.OracleConnString)
	maskedJWTSecret := "*** MASKED ***"
	if cfg.JWTSecret == "default-secret" {
		maskedJWTSecret = "default-secret (!!! WARNING: Using default JWT secret !!!)"
	} else if len(cfg.JWTSecret) < 8 && len(cfg.JWTSecret) > 0 {
		maskedJWTSecret = fmt.Sprintf("*** MASKED (short: %d chars) ***", len(cfg.JWTSecret))
	} else if cfg.JWTSecret == "" {
		maskedJWTSecret = "--- EMPTY (!!! WARNING: JWT Secret is empty !!!) ---"
	}
	fields := []zapcore.Field{
		zap.String("AppEnv", cfg.AppEnv),
		zap.String("Port", cfg.Port),
		zap.Bool("Prefork", cfg.Prefork),
		zap.String("JWTSecret", maskedJWTSecret),
		zap.String("OracleConnString", maskedOracleConn),
		zap.Int("OracleMaxPoolOpenConns", cfg.OracleMaxPoolOpenConns),
		zap.Int("OracleMaxPoolIdleConns", cfg.OracleMaxPoolIdleConns),
		zap.Int("OracleMaxPoolConnLifetimeMinutes", cfg.OracleMaxPoolConnLifetimeMinutes),
		zap.Int("OracleMaxPoolConnIdleTimeMinutes", cfg.OracleMaxPoolConnIdleTimeMinutes),
		zap.String("SQLiteDBPath", cfg.SQLiteDBPath),
		zap.String("LogFilePath", cfg.LogFilePath),
		zap.String("LogLevel", cfg.LogLevel),
		zap.Int("LogRotateIntervalHours", cfg.LogRotateInterval),
		zap.Int("LogMaxSizeMB", cfg.LogMaxSize),
		zap.Int("LogMaxBackups", cfg.LogMaxBackups),
		zap.Int("LogMaxAgeDays", cfg.LogMaxAge),
		zap.Bool("LogCompress", cfg.LogCompress),
		zap.Duration("LogProcessor_BatchInterval", cfg.LogBatchInterval),
		zap.Int("LogProcessor_BatchSize", cfg.LogProcessorBatchSize),
		zap.Int("LogProcessor_OracleRetryAttempts", cfg.LogProcessorOracleRetryAttempts),
		zap.Int("LogProcessor_OracleRetryDelaySeconds", cfg.LogProcessorOracleRetryDelaySeconds),
		zap.String("UploadDir", cfg.UploadDir),
		zap.String("CORS_AllowOrigins", cfg.CORSAllowOrigins),
		zap.String("CORS_AllowMethods", cfg.CORSAllowMethods),
		zap.String("CORS_AllowHeaders", cfg.CORSAllowHeaders),
		zap.Bool("DedicatedSQLiteLog_Enabled", cfg.SQLLiteLogEnabled),
		zap.String("DedicatedSQLiteLog_Level", cfg.SQLLiteLogLevel),
	}
	logger.Debug("Loaded application configuration details", fields...)
}
