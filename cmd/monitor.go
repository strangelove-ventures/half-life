package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/staking4all/celestia-monitoring-bot/services/db"
	"github.com/staking4all/celestia-monitoring-bot/services/models"
	"github.com/staking4all/celestia-monitoring-bot/services/monitor"
	"github.com/staking4all/celestia-monitoring-bot/services/telegram"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Daemon to monitor validators",
	Long:  "Monitors validators and pushes alerts to Telegram",
	Args:  cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("file")
		dat, err := os.ReadFile(configFile)
		if err != nil {
			zap.L().Error("Error reading config.yaml", zap.Error(err))
			return err
		}

		config := models.Config{}
		err = yaml.Unmarshal(dat, &config)
		if err != nil {
			zap.L().Error("Error parsing config.yaml", zap.Error(err))
			return err
		}
		config.LoadDefault()

		tn, err := telegram.NewTelegramNotificationService(config)
		if err != nil {
			zap.L().Error("error starting telegram notification", zap.Error(err))
			return err
		}

		db, err := db.NewDB()
		if err != nil {
			zap.L().Error("error starting database", zap.Error(err))
			return err
		}
		defer db.Close()

		m, err := monitor.NewMonitorService(config, tn, db)
		if err != nil {
			zap.L().Error("error stating monitor", zap.Error(err))
			return err
		}
		defer func() {
			_ = m.Stop()
		}()

		err = m.Run()
		if err != nil {
			zap.L().Error("monitor running", zap.Error(err))
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
	monitorCmd.Flags().StringP("file", "f", "./config.yaml", "File path to config yaml")
}
