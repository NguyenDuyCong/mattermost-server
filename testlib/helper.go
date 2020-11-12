// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package testlib

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/services/searchengine"
	"github.com/mattermost/mattermost-server/v5/store"
	"github.com/mattermost/mattermost-server/v5/store/searchlayer"
	"github.com/mattermost/mattermost-server/v5/store/sqlstore"
	"github.com/mattermost/mattermost-server/v5/store/storetest"
	"github.com/mattermost/mattermost-server/v5/utils"
)

type MainHelper struct {
	Settings         *model.SqlSettings
	Store            store.Store
	SearchEngine     *searchengine.Broker
	SQLSupplier      *sqlstore.SqlSupplier
	ClusterInterface *FakeClusterInterface

	status           int
	testResourcePath string
}

type HelperOptions struct {
	EnableStore     bool
	EnableResources bool
}

func NewMainHelper() *MainHelper {
	return NewMainHelperWithOptions(&HelperOptions{
		EnableStore:     true,
		EnableResources: true,
	})
}

func NewMainHelperWithOptions(options *HelperOptions) *MainHelper {
	var mainHelper MainHelper
	flag.Parse()

	// Setup a global logger to catch tests logging outside of app context
	// The global logger will be stomped by apps initializing but that's fine for testing.
	// Ideally this won't happen.
	mlog.InitGlobalLogger(mlog.NewLogger(&mlog.LoggerConfiguration{
		EnableConsole: true,
		ConsoleJson:   true,
		ConsoleLevel:  "error",
		EnableFile:    false,
	}))

	utils.TranslationsPreInit()

	if options != nil {
		if options.EnableStore && !testing.Short() {
			mainHelper.setupStore()
		}

		if options.EnableResources {
			mainHelper.setupResources()
		}
	}

	return &mainHelper
}

func (h *MainHelper) Main(m *testing.M) {
	if h.testResourcePath != "" {
		prevDir, err := os.Getwd()
		if err != nil {
			panic("Failed to get current working directory: " + err.Error())
		}

		err = os.Chdir(h.testResourcePath)
		if err != nil {
			panic(fmt.Sprintf("Failed to set current working directory to %s: %s", h.testResourcePath, err.Error()))
		}

		defer func() {
			err := os.Chdir(prevDir)
			if err != nil {
				panic(fmt.Sprintf("Failed to restore current working directory to %s: %s", prevDir, err.Error()))
			}
		}()
	}

	h.status = m.Run()
}

func (h *MainHelper) setupStore() {
	driverName := os.Getenv("MM_SQLSETTINGS_DRIVERNAME")
	if driverName == "" {
		driverName = model.DATABASE_DRIVER_POSTGRES
	}

	h.Settings = storetest.MakeSqlSettings(driverName)

	config := &model.Config{}
	config.SetDefaults()

	h.SearchEngine = searchengine.NewBroker(config, nil)
	h.ClusterInterface = &FakeClusterInterface{}
	h.SQLSupplier = sqlstore.NewSqlSupplier(*h.Settings, nil)
	h.Store = searchlayer.NewSearchLayer(&TestStore{
		h.SQLSupplier,
	}, h.SearchEngine, config)
}

func (h *MainHelper) setupResources() {
	var err error
	h.testResourcePath, err = SetupTestResources()
	if err != nil {
		panic("failed to setup test resources: " + err.Error())
	}
}

// PreloadMigrations preloads the migrations and roles into the database
// so that they are not run again when the migrations happen every time
// the server is started.
// This change is forward-compatible with new migrations and only new migrations
// will get executed.
// Only if the schema of either roles or systems table changes, this will break.
// In that case, just update the migrations or comment this out for the time being.
// In the worst case, only an optimization is lost.
//
// Re-generate the files with:
// pg_dump -a -h localhost -U mmuser -d <> --no-comments --inserts -t roles -t systems
// mysqldump -u root -p <> --no-create-info --extended-insert=FALSE Systems Roles
func (h *MainHelper) PreloadMigrations() {
	var buf []byte
	var err error
	switch *h.Settings.DriverName {
	case model.DATABASE_DRIVER_POSTGRES:
		buf, err = ioutil.ReadFile("mattermost-server/testlib/testdata/postgres_migration_warmup.sql")
		if err != nil {
			panic(fmt.Errorf("cannot read file: %v", err))
		}
	case model.DATABASE_DRIVER_MYSQL:
		buf, err = ioutil.ReadFile("mattermost-server/testlib/testdata/mysql_migration_warmup.sql")
		if err != nil {
			panic(fmt.Errorf("cannot read file: %v", err))
		}
	}
	handle := h.SQLSupplier.GetMaster()
	_, err = handle.Exec(string(buf))
	if err != nil {
		mlog.Error("Error preloading migrations. Did the schema change? If yes, then update the warmup files accordingly. Or just comment this method and file a ticket if there's a rush.")
		panic(err)
	}
}

func (h *MainHelper) Close() error {
	if h.SQLSupplier != nil {
		h.SQLSupplier.Close()
	}
	if h.Settings != nil {
		storetest.CleanupSqlSettings(h.Settings)
	}
	if h.testResourcePath != "" {
		os.RemoveAll(h.testResourcePath)
	}

	if r := recover(); r != nil {
		log.Fatalln(r)
	}

	os.Exit(h.status)

	return nil
}

func (h *MainHelper) GetSQLSettings() *model.SqlSettings {
	if h.Settings == nil {
		panic("MainHelper not initialized with database access.")
	}

	return h.Settings
}

func (h *MainHelper) GetStore() store.Store {
	if h.Store == nil {
		panic("MainHelper not initialized with store.")
	}

	return h.Store
}

func (h *MainHelper) GetSQLSupplier() *sqlstore.SqlSupplier {
	if h.SQLSupplier == nil {
		panic("MainHelper not initialized with sql supplier.")
	}

	return h.SQLSupplier
}

func (h *MainHelper) GetClusterInterface() *FakeClusterInterface {
	if h.ClusterInterface == nil {
		panic("MainHelper not initialized with cluster interface.")
	}

	return h.ClusterInterface
}

func (h *MainHelper) GetSearchEngine() *searchengine.Broker {
	if h.SearchEngine == nil {
		panic("MainHelper not initialized with search engine")
	}

	return h.SearchEngine
}
