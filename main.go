package main

import (
	"github.com/staking4all/celestia-monitoring-bot/cmd"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// set log
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	zapCfg.Level.SetLevel(zap.DebugLevel)
	l, err := zapCfg.Build()
	if err != nil {
		zap.L().Panic("Failed to init zap global logger, no zap log will be shown till zap is properly initialized", zap.Error(err))
	}
	zap.ReplaceGlobals(l)

	cmd.Execute()
}
