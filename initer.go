package svc

import (
	"container/list"
	"context"
	"github.com/guestin/log"
	"github.com/guestin/mob/msync"
	"github.com/ooopSnake/assert.go"
	"go.uber.org/zap"
	"strings"
	"sync/atomic"
)

var logger log.ClassicLog

type (
	ExitResult struct {
		Code  int
		Error error
	}
	ExecFunc  func() ExitResult
	InitFunc  func(ctx context.Context) (ExecFunc, error)
	InitFunc2 func(ctx context.Context, moduleName string, zlogger *zap.Logger) (ExecFunc, error)
	Unit      struct {
		Name string
		Func InitFunc2
	}
)

func upgradeInitFuncToV2(f InitFunc) InitFunc2 {
	return func(ctx context.Context, moduleName string, zlogger *zap.Logger) (ExecFunc, error) {
		return f(ctx)
	}
}

var units []Unit

func RegisterUnit(name string, f InitFunc) {
	assert.Must(len(strings.TrimSpace(name)) != 0, "name must not empty or blank").Panic()
	assert.Must(f != nil, "func must not be nil").Panic()
	units = append(units, Unit{
		Name: name,
		Func: upgradeInitFuncToV2(f),
	})
}

func RegisterUnit2(name string, f InitFunc2) {
	assert.Must(len(strings.TrimSpace(name)) != 0, "name must not empty or blank").Panic()
	assert.Must(f != nil, "func must not be nil").Panic()
	units = append(units, Unit{
		Name: name,
		Func: f,
	})
}

func RegisterUnits(inputUnits []Unit) {
	units = append(units, inputUnits...)
}

// noinspection ALL
func Execute(ctx context.Context, zapLoggger *zap.Logger, loggerOpt ...log.Opt) {
	assert.Must(zapLoggger != nil, "no logger setup !!! ").Panic()
	logger = log.NewTaggedClassicLogger(zapLoggger, "bootloader", loggerOpt...)
	if len(units) == 0 {
		logger.Debug("no service,exit...")
		return
	}
	defer log.Flush()
	group := msync.NewAsyncTaskGroup()
	defer group.Wait()
	taskStack := list.New()
	defer func() {
		for taskStack.Len() != 0 {
			item := taskStack.Front()
			taskStack.Remove(item)
			taskItem := item.Value.(*_UnitTask)
			taskItem.Cancel()
			taskItem.Wait()
		}
	}()
	runner := func(unitItem Unit) {
		defer log.Flush()
		logger.With(
			log.UseSubTag(log.NewFixStyleText(unitItem.Name, log.Green, true))).
			Info("start init...")
		task, err := taskWrapper(ctx, zapLoggger, unitItem)
		if err != nil {
			logger.With(
				log.UseSubTag(log.NewFixStyleText(unitItem.Name, log.Red, true))).
				Panic("init failed,err:", err)
			return
		}
		logger.With(
			log.UseSubTag(log.NewFixStyleText(unitItem.Name, log.Yellow, true))).
			Info("init success!")
		taskStack.PushFront(task)
		logger.With(
			log.UseSubTag(log.NewFixStyleText(unitItem.Name, log.Cyan, true))).
			Info("running...")
		group.AddTask(func() {
			defer func() {
				exitPanic := recover()
				if exitPanic != nil {
					logger.With(
						log.UseSubTag(log.NewFixStyleText(unitItem.Name, log.Red, true))).
						Panicf("exit unexpected, panic:%v", exitPanic)
				}
			}()
			result := task.Exec()
			exitTagColor := log.Cyan
			var logMeth = logger.With(
				log.UseSubTag(log.NewFixStyleText(unitItem.Name, exitTagColor, true))).Infof
			if result.Code != 0 {
				exitTagColor = log.Red
				logMeth = logger.With(
					log.UseSubTag(log.NewFixStyleText(unitItem.Name, exitTagColor, true))).Warnf
			}
			logMeth("exit, code: %d,err: %v", result.Code, result.Error)
		})
	}
	for idx := range units {
		runner(units[idx])
	}
	<-ctx.Done()
}

type _UnitTask struct {
	ctx        context.Context
	cancel     context.CancelFunc
	originTask ExecFunc
	done       chan struct{}
	closeOnce  uint32
}

func (this *_UnitTask) HasTask() bool {
	return this.originTask != nil
}

func (this *_UnitTask) Exec() ExitResult {
	defer func() {
		if this.done != nil && atomic.CompareAndSwapUint32(&this.closeOnce, 0, 1) {
			close(this.done)
		}
	}()
	if this.originTask == nil {
		<-this.ctx.Done()
		return NewSuccessResult()
	}
	return this.originTask()
}

func (this *_UnitTask) Wait() {
	<-this.done
}

func (this *_UnitTask) Cancel() {
	this.cancel()
}

func taskWrapper(
	ctx context.Context,
	zlogger *zap.Logger,
	u Unit) (*_UnitTask, error) {
	child, cancelFunc := context.WithCancel(ctx)
	originalExecFunc, err := u.Func(child, u.Name, zlogger)
	if err != nil {
		cancelFunc()
		return nil, err
	}
	if originalExecFunc == nil {
		return &_UnitTask{
			ctx:    child,
			cancel: cancelFunc,
			done:   make(chan struct{}),
		}, nil
	}
	wrap := &_UnitTask{
		ctx:        child,
		cancel:     cancelFunc,
		originTask: originalExecFunc,
		done:       make(chan struct{}),
	}
	return wrap, nil
}
