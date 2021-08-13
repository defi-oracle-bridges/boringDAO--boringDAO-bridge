package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/boringdao/bridge/internal/monitor/bsc"
	matic "github.com/boringdao/bridge/internal/monitor/eth"

	"github.com/boringdao/bridge/internal/loggers"
	"github.com/boringdao/bridge/internal/repo"
	"github.com/boringdao/bridge/pkg/storage"
	"github.com/boringdao/bridge/pkg/storage/leveldb"

	"github.com/common-nighthawk/go-figure"
	"github.com/sirupsen/logrus"
)

type Bridge struct {
	repo     *repo.Repo
	maticMnt *matic.Monitor
	bscMnt   *bsc.Monitor
	storage  storage.Storage
	logger   logrus.FieldLogger
	mux      sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

func New(repoRoot *repo.Repo) (*Bridge, error) {
	storagePath := repo.GetStoragePath(repoRoot.Config.RepoRoot, "app")
	boringStorage, err := leveldb.New(storagePath)
	if err != nil {
		return nil, err
	}

	maticMnt, err := matic.New(repoRoot.Config, loggers.Logger(loggers.MATIC))
	if err != nil {
		return nil, err
	}

	bscMnt, err := bsc.New(repoRoot.Config, loggers.Logger(loggers.BSC))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Bridge{
		repo:     repoRoot,
		maticMnt: maticMnt,
		bscMnt:   bscMnt,
		storage:  boringStorage,
		logger:   loggers.Logger(loggers.APP),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func (b *Bridge) Start() error {
	err := b.maticMnt.Start()
	if err != nil {
		return err
	}

	err = b.bscMnt.Start()
	if err != nil {
		return err
	}

	go b.listenEthCocoC()

	go b.listenBscCocoC()

	b.printLogo()

	return nil
}

func (b *Bridge) Stop() error {
	b.cancel()
	return nil
}

func (b *Bridge) listenEthCocoC() {
	cocoC := b.maticMnt.HandleCocoC()
	for {
		select {
		case coco := <-cocoC:
			handle := func() {
				b.mux.Lock()
				defer b.mux.Unlock()
				b.logger.Infof("========> start handle bsc transaction...")
				defer b.logger.Infof("========> end handle bsc transaction...")
				if b.maticMnt.HasTx(coco.TxId, coco) {
					b.logger.WithField("tx", coco.TxId).Error("has handled the interchain event")
					return
				}
				var err error
				switch coco.Typ {
				case matic.Lock:
					err = b.bscMnt.CrossIn(coco.TxId+"#LOCK", coco.Token1, coco.From, coco.To, coco.ChainID0, coco.Amount)
				case matic.CrossBurn:
					err = b.bscMnt.Unlock(coco.TxId+"#CrossBurn", coco.Token1, coco.From, coco.To, coco.ChainID0, coco.Amount)
				case matic.Rollback:
					err = b.bscMnt.Rollback(coco.TxId+"#Rollback", coco.Token1, coco.From, coco.To, coco.ChainID0, coco.Amount)
				}
				if err != nil {
					b.logger.Panic(err)
				}
				b.maticMnt.PutTxID(coco.TxId, coco)

			}
			handle()
		case <-b.ctx.Done():
			close(cocoC)
			return
		}
	}
}

func (b *Bridge) listenBscCocoC() {
	cocoC := b.bscMnt.HandleCocoC()
	for {
		select {
		case coco := <-cocoC:
			handle := func() {
				b.mux.Lock()
				defer b.mux.Unlock()
				b.logger.Infof("========> start handle eth transaction...")
				defer b.logger.Infof("========> end handle eth transaction...")
				if b.bscMnt.HasTx(coco.TxId, coco) {
					b.logger.WithField("tx", coco.TxId).Error("has handled the interchain event")
					return
				}

				var err error
				switch coco.Typ {
				case bsc.Lock:
					err = b.maticMnt.CrossIn(coco.TxId+"#LOCK", coco.Token1, coco.From, coco.To, coco.ChainID0, coco.Amount)
				case bsc.CrossBurn:
					err = b.maticMnt.Unlock(coco.TxId+"#CrossBurn", coco.Token1, coco.From, coco.To, coco.ChainID0, coco.Amount)
				case bsc.Rollback:
					err = b.maticMnt.Rollback(coco.TxId+"#Rollback", coco.Token1, coco.From, coco.To, coco.ChainID0, coco.Amount)
				}
				if err != nil {
					b.logger.Panic(err)
				}

				b.bscMnt.PutTxID(coco.TxId, coco)

			}
			handle()
		case <-b.ctx.Done():
			close(cocoC)
			return
		}
	}
}

func (b *Bridge) printLogo() {
	for {
		time.Sleep(100 * time.Millisecond)
		fmt.Println()
		fmt.Println("=======================================================")
		fig := figure.NewColorFigure("Bridge", "slant", "red", true)
		fig.Print()
		fmt.Println()
		fmt.Println("=======================================================")
		fmt.Println()
		return
	}
}
