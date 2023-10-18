package tests

import (
	"context"
	"github.com/guestin/log"
	"github.com/guestin/svc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"testing"
	"time"
)

func TestExecute(t *testing.T) {
	rootLogger, _ := log.EasyInitConsoleLogger(zapcore.DebugLevel, zapcore.ErrorLevel)
	testCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// use `RegisterUnit2`
	svc.RegisterUnit2("u1",
		func(ctx context.Context, moduleName string, zlogger *zap.Logger) (svc.ExecFunc, error) {
			zlogger.Info("this is init stage", zap.String("moduleName", moduleName))
			return func() svc.ExitResult {
				zlogger.Info("this is execute stage", zap.String("moduleName", moduleName))
				<-ctx.Done()
				return svc.NewSuccessResult()
			}, nil
		})
	// simple declare
	AutoCancelUnit := func(ctx context.Context, moduleName string, zlogger *zap.Logger) (svc.ExecFunc, error) {
		zlogger.Info("will cancel after 3s", zap.String("moduleName", moduleName))
		return func() svc.ExitResult {
			time.Sleep(time.Second * 3)
			cancel()
			return svc.NewSuccessResult()
		}, nil
	}
	// example for unit table
	allUnits := []svc.Unit{
		{
			Name: "auto cancel",
			Func: AutoCancelUnit,
		},
	}
	// reg all units
	svc.RegisterUnits(allUnits)
	svc.Execute(testCtx, rootLogger)
}
