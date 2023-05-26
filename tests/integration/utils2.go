// Copyright 2022 Democratized Data Foundation
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package tests

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/sourcenetwork/immutable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcenetwork/defradb/client"
	"github.com/sourcenetwork/defradb/config"
	"github.com/sourcenetwork/defradb/datastore"
	badgerds "github.com/sourcenetwork/defradb/datastore/badger/v3"
	"github.com/sourcenetwork/defradb/datastore/memory"
	"github.com/sourcenetwork/defradb/db"
	"github.com/sourcenetwork/defradb/errors"
	"github.com/sourcenetwork/defradb/logging"
	"github.com/sourcenetwork/defradb/node"
)

const (
	memoryBadgerEnvName        = "DEFRA_BADGER_MEMORY"
	fileBadgerEnvName          = "DEFRA_BADGER_FILE"
	fileBadgerPathEnvName      = "DEFRA_BADGER_FILE_PATH"
	rootDBFilePathEnvName      = "DEFRA_TEST_ROOT"
	inMemoryEnvName            = "DEFRA_IN_MEMORY"
	setupOnlyEnvName           = "DEFRA_SETUP_ONLY"
	detectDbChangesEnvName     = "DEFRA_DETECT_DATABASE_CHANGES"
	repositoryEnvName          = "DEFRA_CODE_REPOSITORY"
	targetBranchEnvName        = "DEFRA_TARGET_BRANCH"
	documentationDirectoryName = "data_format_changes"
)

type DatabaseType string

const (
	badgerIMType   DatabaseType = "badger-in-memory"
	defraIMType    DatabaseType = "defra-memory-datastore"
	badgerFileType DatabaseType = "badger-file-system"
)

var (
	log            = logging.MustNewLogger("tests.integration")
	badgerInMemory bool
	badgerFile     bool
	inMemoryStore  bool
)

const subscriptionTimeout = 1 * time.Second

var databaseDir string
var rootDatabaseDir string

/*
If this is set to true the integration test suite will instead of its normal profile do
the following:

On [package] Init:
  - Get the (local) latest commit from the target/parent branch // code assumes
    git fetch has been done
  - Check to see if a clone of that commit/branch is available in the temp dir, and
    if not clone the target branch
  - Check to see if there are any new .md files in the current branch's data_format_changes
    dir (vs the target branch)

For each test:
  - If new documentation detected, pass the test and exit
  - Create a new (test/auto-deleted) temp dir for defra to live/run in
  - Run the test setup (add initial schema, docs, updates) using the target branch (test is skipped
    if test does not exist in target and is new to this branch)
  - Run the test request and assert results (as per normal tests) using the current branch
*/
var DetectDbChanges bool
var SetupOnly bool

var detectDbChangesCodeDir string
var areDatabaseFormatChangesDocumented bool
var previousTestCaseTestName string

func init() {
	// We use environment variables instead of flags `go test ./...` throws for all packages
	//  that don't have the flag defined
	badgerFileValue, _ := os.LookupEnv(fileBadgerEnvName)
	badgerInMemoryValue, _ := os.LookupEnv(memoryBadgerEnvName)
	databaseDir, _ = os.LookupEnv(fileBadgerPathEnvName)
	rootDatabaseDir, _ = os.LookupEnv(rootDBFilePathEnvName)
	detectDbChangesValue, _ := os.LookupEnv(detectDbChangesEnvName)
	inMemoryStoreValue, _ := os.LookupEnv(inMemoryEnvName)
	repositoryValue, repositorySpecified := os.LookupEnv(repositoryEnvName)
	setupOnlyValue, _ := os.LookupEnv(setupOnlyEnvName)
	targetBranchValue, targetBranchSpecified := os.LookupEnv(targetBranchEnvName)

	badgerFile = getBool(badgerFileValue)
	badgerInMemory = getBool(badgerInMemoryValue)
	inMemoryStore = getBool(inMemoryStoreValue)
	DetectDbChanges = getBool(detectDbChangesValue)
	SetupOnly = getBool(setupOnlyValue)

	if !repositorySpecified {
		repositoryValue = "https://github.com/sourcenetwork/defradb.git"
	}

	if !targetBranchSpecified {
		targetBranchValue = "develop"
	}

	// default is to run against all
	if !badgerInMemory && !badgerFile && !inMemoryStore && !DetectDbChanges {
		badgerInMemory = true
		// Testing against the file system is off by default
		badgerFile = false
		inMemoryStore = true
	}

	if DetectDbChanges {
		detectDbChangesInit(repositoryValue, targetBranchValue)
	}
}

func getBool(val string) bool {
	switch strings.ToLower(val) {
	case "true":
		return true
	default:
		return false
	}
}

// AssertPanicAndSkipChangeDetection asserts that the code of function actually panics,
//
//	also ensures the change detection is skipped so no false fails happen.
//
//	Usage: AssertPanicAndSkipChangeDetection(t, func() { executeTestCase(t, test) })
func AssertPanicAndSkipChangeDetection(t *testing.T, f assert.PanicTestFunc) bool {
	if IsDetectingDbChanges() {
		// The `assert.Panics` call will falsely fail if this test is executed during
		// a detect changes test run
		t.Skip()
	}
	return assert.Panics(t, f, "expected a panic, but none found.")
}

func NewBadgerMemoryDB(ctx context.Context, dbopts ...db.Option) (client.DB, error) {
	opts := badgerds.Options{Options: badger.DefaultOptions("").WithInMemory(true)}
	rootstore, err := badgerds.NewDatastore("", &opts)
	if err != nil {
		return nil, err
	}

	dbopts = append(dbopts, db.WithUpdateEvents())

	db, err := db.NewDB(ctx, rootstore, dbopts...)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func NewInMemoryDB(ctx context.Context) (client.DB, error) {
	rootstore := memory.NewDatastore(ctx)
	db, err := db.NewDB(ctx, rootstore, db.WithUpdateEvents())
	if err != nil {
		return nil, err
	}

	return db, nil
}

func NewBadgerFileDB(ctx context.Context, t testing.TB) (client.DB, string, error) {
	var dbPath string
	if databaseDir != "" {
		dbPath = databaseDir
	} else if rootDatabaseDir != "" {
		dbPath = path.Join(rootDatabaseDir, t.Name())
	} else {
		dbPath = t.TempDir()
	}

	db, err := newBadgerFileDB(ctx, t, dbPath)
	return db, dbPath, err
}

func newBadgerFileDB(ctx context.Context, t testing.TB, path string) (client.DB, error) {
	opts := badgerds.Options{Options: badger.DefaultOptions(path)}
	rootstore, err := badgerds.NewDatastore(path, &opts)
	if err != nil {
		return nil, err
	}

	db, err := db.NewDB(ctx, rootstore, db.WithUpdateEvents())
	if err != nil {
		return nil, err
	}

	return db, nil
}

func GetDatabaseTypes() []DatabaseType {
	databases := []DatabaseType{}

	if badgerInMemory {
		databases = append(databases, badgerIMType)
	}

	if badgerFile {
		databases = append(databases, badgerFileType)
	}

	if inMemoryStore {
		databases = append(databases, defraIMType)
	}

	return databases
}

func GetDatabase(ctx context.Context, t *testing.T, dbt DatabaseType) (client.DB, string, error) {
	switch dbt {
	case badgerIMType:
		db, err := NewBadgerMemoryDB(ctx, db.WithUpdateEvents())
		if err != nil {
			return nil, "", err
		}
		return db, "", nil

	case badgerFileType:
		db, path, err := NewBadgerFileDB(ctx, t)
		if err != nil {
			return nil, "", err
		}
		return db, path, nil

	case defraIMType:
		db, err := NewInMemoryDB(ctx)
		if err != nil {
			return nil, "", err
		}
		return db, "", nil
	}

	return nil, "", nil
}

// ExecuteTestCase executes the given TestCase against the configured database
// instances.
//
// Will also attempt to detect incompatible changes in the persisted data if
// configured to do so (the CI will do so, but disabled by default as it is slow).
func ExecuteTestCase(
	t *testing.T,
	collectionNames []string,
	testCase TestCase,
) {
	if DetectDbChanges && DetectDbChangesPreTestChecks(t, collectionNames) {
		return
	}

	ctx := context.Background()
	dbts := GetDatabaseTypes()
	// Assert that this is not empty to protect against accidental mis-configurations,
	// otherwise an empty set would silently pass all the tests.
	require.NotEmpty(t, dbts)

	for _, dbt := range dbts {
		executeTestCase(ctx, t, collectionNames, testCase, dbt)
	}
}

func executeTestCase(
	ctx context.Context,
	t *testing.T,
	collectionNames []string,
	testCase TestCase,
	dbt DatabaseType,
) {
	var done bool
	log.Info(ctx, testCase.Description, logging.NewKV("Database", dbt))

	flattenActions(&testCase)
	startActionIndex, endActionIndex := getActionRange(testCase)
	txns := []datastore.Txn{}
	allActionsDone := make(chan struct{})
	resultsChans := []chan func(){}
	syncChans := []chan struct{}{}
	nodeAddresses := []string{}
	// The actions responsible for configuring the node
	nodeConfigs := []config.Config{}
	nodes, dbPaths := getStartingNodes(ctx, t, dbt, testCase)
	// It is very important that the databases are always closed, otherwise resources will leak
	// as tests run.  This is particularly important for file based datastores.
	defer closeNodes(ctx, t, nodes)

	// Documents and Collections may already exist in the database if actions have been split
	// by the change detector so we should fetch them here at the start too (if they exist).
	// collections are by node (index), as they are specific to nodes.
	collections := getCollections(ctx, t, nodes, collectionNames)
	// documents are by collection (index), these are not node specific.
	documents := getDocuments(ctx, t, testCase, collections, startActionIndex)

	for i := startActionIndex; i <= endActionIndex; i++ {
		// declare default database for ease of use
		var db client.DB
		if len(nodes) > 0 {
			db = nodes[0].DB
		}

		switch action := testCase.Actions[i].(type) {
		case ConfigureNode:
			if DetectDbChanges {
				// We do not yet support the change detector for tests running across multiple nodes.
				t.SkipNow()
				return
			}
			cfg := action()
			node, address, path := configureNode(ctx, t, dbt, cfg)
			nodes = append(nodes, node)
			nodeAddresses = append(nodeAddresses, address)
			dbPaths = append(dbPaths, path)
			nodeConfigs = append(nodeConfigs, cfg)

		case Restart:
			// Append the new syncChans on top of the previous - the old syncChans will be closed
			// gracefully as part of the node closure.
			syncChans = append(
				syncChans,
				restartNodes(ctx, t, testCase, dbt, i, nodes, dbPaths, nodeAddresses, nodeConfigs)...,
			)

			// If the db was restarted we need to refresh the collection definitions as the old instances
			// will reference the old (closed) database instances.
			collections = getCollections(ctx, t, nodes, collectionNames)

		case ConnectPeers:
			syncChans = append(syncChans, connectPeers(ctx, t, testCase, action, nodes, nodeAddresses))

		case ConfigureReplicator:
			syncChans = append(syncChans, configureReplicator(ctx, t, testCase, action, nodes, nodeAddresses))

		case SubscribeToCollection:
			subscribeToCollection(ctx, t, testCase, action, nodes, collections)

		case UnsubscribeToCollection:
			unsubscribeToCollection(ctx, t, testCase, action, nodes, collections)

		case GetAllP2PCollections:
			getAllP2PCollections(ctx, t, action, nodes, collections)

		case SchemaUpdate:
			updateSchema(ctx, t, nodes, testCase, action)
			// If the schema was updated we need to refresh the collection definitions.
			collections = getCollections(ctx, t, nodes, collectionNames)

		case SchemaPatch:
			patchSchema(ctx, t, nodes, testCase, action)
			// If the schema was updated we need to refresh the collection definitions.
			collections = getCollections(ctx, t, nodes, collectionNames)

		case CreateDoc:
			documents = createDoc(ctx, t, testCase, nodes, collections, documents, action)

		case DeleteDoc:
			deleteDoc(ctx, t, testCase, nodes, collections, documents, action)

		case UpdateDoc:
			updateDoc(ctx, t, testCase, nodes, collections, documents, action)

		case TransactionRequest2:
			txns = executeTransactionRequest(ctx, t, db, txns, testCase, action)

		case TransactionCommit:
			commitTransaction(ctx, t, txns, testCase, action)

		case SubscriptionRequest:
			var resultsChan chan func()
			resultsChan, done = executeSubscriptionRequest(ctx, t, allActionsDone, db, testCase, action)
			if done {
				return
			}
			resultsChans = append(resultsChans, resultsChan)

		case Request:
			executeRequest(ctx, t, nodes, testCase, action)

		case IntrospectionRequest:
			assertIntrospectionResults(ctx, t, testCase.Description, db, action)

		case ClientIntrospectionRequest:
			assertClientIntrospectionResults(ctx, t, testCase.Description, db, action)

		case WaitForSync:
			waitForSync(t, testCase, action, syncChans)

		case SetupComplete:
			// no-op, just continue.

		default:
			t.Fatalf("Unknown action type %T", action)
		}
	}

	// Notify any active subscriptions that all requests have been sent.
	close(allActionsDone)

	for _, resultsChan := range resultsChans {
		select {
		case subscriptionAssert := <-resultsChan:
			// We want to assert back in the main thread so failures get recorded properly
			subscriptionAssert()

		// a safety in case the stream hangs - we don't want the tests to run forever.
		case <-time.After(subscriptionTimeout):
			assert.Fail(t, "timeout occurred while waiting for data stream", testCase.Description)
		}
	}
}

// closeNodes closes all the given nodes, ensuring that resources are properly released.
func closeNodes(
	ctx context.Context,
	t *testing.T,
	nodes []*node.Node,
) {
	for _, node := range nodes {
		if node.Peer != nil {
			err := node.Close()
			require.NoError(t, err)
		}
		node.DB.Close(ctx)
	}
}

// getNodes gets the set of applicable nodes for the given nodeID.
//
// If nodeID has a value it will return that node only, otherwise all nodes will be returned.
func getNodes(nodeID immutable.Option[int], nodes []*node.Node) []*node.Node {
	if !nodeID.HasValue() {
		return nodes
	}

	return []*node.Node{nodes[nodeID.Value()]}
}

// getNodeCollections gets the set of applicable collections for the given nodeID.
//
// If nodeID has a value it will return collections for that node only, otherwise all collections across all
// nodes will be returned.
func getNodeCollections(nodeID immutable.Option[int], collections [][]client.Collection) [][]client.Collection {
	if !nodeID.HasValue() {
		return collections
	}

	return [][]client.Collection{collections[nodeID.Value()]}
}

func calculateLenForFlattenedActions(testCase *TestCase) int {
	newLen := 0
	for _, a := range testCase.Actions {
		actionGroup := reflect.ValueOf(a)
		switch actionGroup.Kind() {
		case reflect.Array, reflect.Slice:
			newLen += actionGroup.Len()
		default:
			newLen++
		}
	}
	return newLen
}

func flattenActions(testCase *TestCase) {
	newLen := calculateLenForFlattenedActions(testCase)
	if newLen == len(testCase.Actions) {
		return
	}
	newActions := make([]any, 0, newLen)

	for _, a := range testCase.Actions {
		actionGroup := reflect.ValueOf(a)
		switch actionGroup.Kind() {
		case reflect.Array, reflect.Slice:
			for i := 0; i < actionGroup.Len(); i++ {
				newActions = append(
					newActions,
					actionGroup.Index(i).Interface(),
				)
			}
		default:
			newActions = append(newActions, a)
		}
	}
	testCase.Actions = newActions
}

// getActionRange returns the index of the first action to be run, and the last.
//
// Not all processes will run all actions - if this is a change detector run they
// will be split.
//
// If a SetupComplete action is provided, the actions will be split there, if not
// they will be split at the first non SchemaUpdate/CreateDoc/UpdateDoc action.
func getActionRange(testCase TestCase) (int, int) {
	startIndex := 0
	endIndex := len(testCase.Actions) - 1

	if !DetectDbChanges {
		return startIndex, endIndex
	}

	setupCompleteIndex := -1
	firstNonSetupIndex := -1

ActionLoop:
	for i := range testCase.Actions {
		switch testCase.Actions[i].(type) {
		case SetupComplete:
			setupCompleteIndex = i
			// We don't care about anything else if this has been explicitly provided
			break ActionLoop

		case SchemaUpdate, CreateDoc, UpdateDoc, Restart:
			continue

		default:
			firstNonSetupIndex = i
			break ActionLoop
		}
	}

	if SetupOnly {
		if setupCompleteIndex > -1 {
			endIndex = setupCompleteIndex
		} else if firstNonSetupIndex > -1 {
			// -1 to exclude this index
			endIndex = firstNonSetupIndex - 1
		}
	} else {
		if setupCompleteIndex > -1 {
			// +1 to exclude the SetupComplete action
			startIndex = setupCompleteIndex + 1
		} else if firstNonSetupIndex > -1 {
			// We must not set this to -1 :)
			startIndex = firstNonSetupIndex
		} else {
			// if we don't have any non-mutation actions, just use the last action
			startIndex = endIndex
		}
	}

	return startIndex, endIndex
}

// getStartingNodes returns a set of initial Defra nodes for the test to execute against.
//
// If a node(s) has been explicitly configured via a `ConfigureNode` action then an empty
// set will be returned.
func getStartingNodes(
	ctx context.Context,
	t *testing.T,
	dbt DatabaseType,
	testCase TestCase,
) ([]*node.Node, []string) {
	hasExplicitNode := false
	for _, action := range testCase.Actions {
		switch action.(type) {
		case ConfigureNode:
			hasExplicitNode = true
		}
	}

	// If nodes have not been explicitly configured via actions, setup a default one.
	if !hasExplicitNode {
		db, path, err := GetDatabase(ctx, t, dbt)
		require.Nil(t, err)

		return []*node.Node{
				{
					DB: db,
				},
			}, []string{
				path,
			}
	}

	return []*node.Node{}, []string{}
}

func restartNodes(
	ctx context.Context,
	t *testing.T,
	testCase TestCase,
	dbt DatabaseType,
	actionIndex int,
	nodes []*node.Node,
	dbPaths []string,
	nodeAddresses []string,
	configureActions []config.Config,
) []chan struct{} {
	if dbt == badgerIMType || dbt == defraIMType {
		return nil
	}
	closeNodes(ctx, t, nodes)

	// We need to restart the nodes in reverse order, to avoid dial backoff issues.
	for i := len(nodes) - 1; i >= 0; i-- {
		originalPath := databaseDir
		databaseDir = dbPaths[i]
		db, _, err := GetDatabase(ctx, t, dbt)
		require.Nil(t, err)
		databaseDir = originalPath

		if len(configureActions) == 0 {
			// If there are no explicit node configuration actions the node will be
			// basic (i.e. no P2P stuff) and can be yielded now.
			nodes[i] = &node.Node{
				DB: db,
			}
			continue
		}

		cfg := configureActions[i]
		// We need to make sure the node is configured with its old address, otherwise
		// a new one may be selected and reconnnection to it will fail.
		cfg.Net.P2PAddress = strings.Split(nodeAddresses[i], "/p2p/")[0]
		var n *node.Node
		n, err = node.NewNode(
			ctx,
			db,
			cfg.NodeConfig(),
		)
		require.NoError(t, err)

		if err := n.Start(); err != nil {
			closeErr := n.Close()
			if closeErr != nil {
				t.Fatal(fmt.Sprintf("unable to start P2P listeners: %v: problem closing node", err), closeErr)
			}
			require.NoError(t, err)
		}

		nodes[i] = n
	}

	// The index of the action after the last wait action before the current restart action.
	// We wish to resume the wait clock from this point onwards.
	waitGroupStartIndex := 0
actionLoop:
	for i := actionIndex; i >= 0; i-- {
		switch testCase.Actions[i].(type) {
		case WaitForSync:
			// +1 as we do not wish to resume from the wait itself, but the next action
			// following it. This may be the current restart action.
			waitGroupStartIndex = i + 1
			break actionLoop
		}
	}

	syncChans := []chan struct{}{}
	for _, tc := range testCase.Actions {
		switch action := tc.(type) {
		case ConnectPeers:
			// Give the nodes a chance to connect to each other and learn about each other's subscribed topics.
			time.Sleep(100 * time.Millisecond)
			syncChans = append(syncChans, setupPeerWaitSync(
				ctx, t, testCase, waitGroupStartIndex, action, nodes[action.SourceNodeID], nodes[action.TargetNodeID],
			))
		case ConfigureReplicator:
			// Give the nodes a chance to connect to each other and learn about each other's subscribed topics.
			time.Sleep(100 * time.Millisecond)
			syncChans = append(syncChans, setupRepicatorWaitSync(
				ctx, t, testCase, waitGroupStartIndex, action, nodes[action.SourceNodeID], nodes[action.TargetNodeID],
			))
		}
	}

	return syncChans
}

// getCollections returns all the collections of the given names, preserving order.
//
// If a given collection is not present in the database the value at the corresponding
// result-index will be nil.
func getCollections(
	ctx context.Context,
	t *testing.T,
	nodes []*node.Node,
	collectionNames []string,
) [][]client.Collection {
	collections := make([][]client.Collection, len(nodes))

	for nodeID, node := range nodes {
		collections[nodeID] = make([]client.Collection, len(collectionNames))
		allCollections, err := node.DB.GetAllCollections(ctx)
		require.Nil(t, err)

		for i, collectionName := range collectionNames {
			for _, collection := range allCollections {
				if collection.Name() == collectionName {
					collections[nodeID][i] = collection
					break
				}
			}
		}
	}
	return collections
}

// configureNode configures and starts a new Defra node using the provided configuration.
//
// It returns the new node, and its peer address. Any errors generated during configuration
// will result in a test failure.
func configureNode(
	ctx context.Context,
	t *testing.T,
	dbt DatabaseType,
	cfg config.Config,
) (*node.Node, string, string) {
	// WARNING: This is a horrible hack both deduplicates/randomizes peer IDs
	// And affects where libp2p(?) stores some values on the file system, even when using
	// an in memory store.
	cfg.Datastore.Badger.Path = t.TempDir()

	db, path, err := GetDatabase(ctx, t, dbt) //disable change dector, or allow it?
	require.NoError(t, err)

	var n *node.Node
	log.Info(ctx, "Starting P2P node", logging.NewKV("P2P address", cfg.Net.P2PAddress))
	n, err = node.NewNode(
		ctx,
		db,
		cfg.NodeConfig(),
	)
	require.NoError(t, err)

	if err := n.Start(); err != nil {
		closeErr := n.Close()
		if closeErr != nil {
			t.Fatal(fmt.Sprintf("unable to start P2P listeners: %v: problem closing node", err), closeErr)
		}
		require.NoError(t, err)
	}

	address := fmt.Sprintf("%s/p2p/%s", n.ListenAddrs()[0].String(), n.PeerID())

	return n, address, path
}

func getDocuments(
	ctx context.Context,
	t *testing.T,
	testCase TestCase,
	collections [][]client.Collection,
	startActionIndex int,
) [][]*client.Document {
	if len(collections) == 0 {
		// This should only be possible at the moment for P2P testing, for which the
		// change detector is currently disabled.  We'll likely need some fancier logic
		// here if/when we wish to enable it.
		return [][]*client.Document{}
	}

	// For now just do the initial setup using the collections on the first node,
	// this may need to become more involved at a later date depending on testing
	// requirements.
	documentsByCollection := make([][]*client.Document, len(collections[0]))

	for i := range collections[0] {
		documentsByCollection[i] = []*client.Document{}
	}

	for i := 0; i < startActionIndex; i++ {
		switch action := testCase.Actions[i].(type) {
		case CreateDoc:
			// We need to add the existing documents in the order in which the test case lists them
			// otherwise they cannot be referenced correctly by other actions.
			doc, err := client.NewDocFromJSON([]byte(action.Doc))
			if err != nil {
				// If an err has been returned, ignore it - it may be expected and if not
				// the test will fail later anyway
				continue
			}

			// Just use the collection from the first relevant node, as all will be the same for this
			// purpose.
			collection := getNodeCollections(action.NodeID, collections)[0][action.CollectionID]

			// The document may have been mutated by other actions, so to be sure we have the latest
			// version without having to worry about the individual update mechanics we fetch it.
			doc, err = collection.Get(ctx, doc.Key(), false)
			if err != nil {
				// If an err has been returned, ignore it - it may be expected and if not
				// the test will fail later anyway
				continue
			}

			documentsByCollection[action.CollectionID] = append(documentsByCollection[action.CollectionID], doc)
		}
	}

	return documentsByCollection
}

// updateSchema updates the schema using the given details.
func updateSchema(
	ctx context.Context,
	t *testing.T,
	nodes []*node.Node,
	testCase TestCase,
	action SchemaUpdate,
) {
	for _, node := range getNodes(action.NodeID, nodes) {
		_, err := node.DB.AddSchema(ctx, action.Schema)
		expectedErrorRaised := AssertError(t, testCase.Description, err, action.ExpectedError)

		assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
	}
}

func patchSchema(
	ctx context.Context,
	t *testing.T,
	nodes []*node.Node,
	testCase TestCase,
	action SchemaPatch,
) {
	for _, node := range getNodes(action.NodeID, nodes) {
		err := node.DB.PatchSchema(ctx, action.Patch)
		expectedErrorRaised := AssertError(t, testCase.Description, err, action.ExpectedError)

		assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
	}
}

// createDoc creates a document using the collection api and caches it in the
// given documents slice.
func createDoc(
	ctx context.Context,
	t *testing.T,
	testCase TestCase,
	nodes []*node.Node,
	nodeCollections [][]client.Collection,
	documents [][]*client.Document,
	action CreateDoc,
) [][]*client.Document {
	// All the docs should be identical, and we only need 1 copy so taking the last
	// is okay.
	var doc *client.Document
	actionNodes := getNodes(action.NodeID, nodes)
	for nodeID, collections := range getNodeCollections(action.NodeID, nodeCollections) {
		var err error
		doc, err = client.NewDocFromJSON([]byte(action.Doc))
		if AssertError(t, testCase.Description, err, action.ExpectedError) {
			return nil
		}

		err = withRetry(
			actionNodes,
			nodeID,
			func() error { return collections[action.CollectionID].Save(ctx, doc) },
		)
		if AssertError(t, testCase.Description, err, action.ExpectedError) {
			return nil
		}
	}

	assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, false)

	if action.CollectionID >= len(documents) {
		// Expand the slice if required, so that the document can be accessed by collection index
		documents = append(documents, make([][]*client.Document, action.CollectionID-len(documents)+1)...)
	}
	documents[action.CollectionID] = append(documents[action.CollectionID], doc)

	return documents
}

// deleteDoc deletes a document using the collection api and caches it in the
// given documents slice.
func deleteDoc(
	ctx context.Context,
	t *testing.T,
	testCase TestCase,
	nodes []*node.Node,
	nodeCollections [][]client.Collection,
	documents [][]*client.Document,
	action DeleteDoc,
) {
	doc := documents[action.CollectionID][action.DocID]

	var expectedErrorRaised bool
	actionNodes := getNodes(action.NodeID, nodes)
	for nodeID, collections := range getNodeCollections(action.NodeID, nodeCollections) {
		err := withRetry(
			actionNodes,
			nodeID,
			func() error {
				_, err := collections[action.CollectionID].DeleteWithKey(ctx, doc.Key())
				return err
			},
		)
		expectedErrorRaised = AssertError(t, testCase.Description, err, action.ExpectedError)
	}

	assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
}

// updateDoc updates a document using the collection api.
func updateDoc(
	ctx context.Context,
	t *testing.T,
	testCase TestCase,
	nodes []*node.Node,
	nodeCollections [][]client.Collection,
	documents [][]*client.Document,
	action UpdateDoc,
) {
	doc := documents[action.CollectionID][action.DocID]

	err := doc.SetWithJSON([]byte(action.Doc))
	if AssertError(t, testCase.Description, err, action.ExpectedError) {
		return
	}

	var expectedErrorRaised bool
	actionNodes := getNodes(action.NodeID, nodes)
	for nodeID, collections := range getNodeCollections(action.NodeID, nodeCollections) {
		err := withRetry(
			actionNodes,
			nodeID,
			func() error { return collections[action.CollectionID].Save(ctx, doc) },
		)
		expectedErrorRaised = AssertError(t, testCase.Description, err, action.ExpectedError)
	}

	assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
}

// withRetry attempts to perform the given action, retrying up to a DB-defined
// maximum attempt count if a transaction conflict error is returned.
//
// If a P2P-sync commit for the given document is already in progress this
// Save call can fail as the transaction will conflict. We dont want to worry
// about this in our tests so we just retry a few times until it works (or the
// retry limit is breached - important incase this is a different error)
func withRetry(
	nodes []*node.Node,
	nodeID int,
	action func() error,
) error {
	for i := 0; i < nodes[nodeID].MaxTxnRetries(); i++ {
		err := action()
		if err != nil && errors.Is(err, badgerds.ErrTxnConflict) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
	return nil
}

// executeTransactionRequest executes the given transactional request.
//
// It will create and cache a new transaction if it is the first of the given
// TransactionId. If an error is returned the transaction will be discarded before
// this function returns.
func executeTransactionRequest(
	ctx context.Context,
	t *testing.T,
	db client.DB,
	txns []datastore.Txn,
	testCase TestCase,
	action TransactionRequest2,
) []datastore.Txn {
	if action.TransactionID >= len(txns) {
		// Extend the txn slice so this txn can fit and be accessed by TransactionId
		txns = append(txns, make([]datastore.Txn, action.TransactionID-len(txns)+1)...)
	}

	if txns[action.TransactionID] == nil {
		// Create a new transaction if one does not already exist.
		txn, err := db.NewTxn(ctx, false)
		if AssertError(t, testCase.Description, err, action.ExpectedError) {
			txn.Discard(ctx)
			return nil
		}

		txns[action.TransactionID] = txn
	}

	result := db.WithTxn(txns[action.TransactionID]).ExecRequest(ctx, action.Request)
	expectedErrorRaised := assertRequestResults(
		ctx,
		t,
		testCase.Description,
		&result.GQL,
		action.Results,
		action.ExpectedError,
		// anyof is not yet supported by transactional requests
		0,
		map[docFieldKey][]any{},
	)

	assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)

	if expectedErrorRaised {
		// Make sure to discard the transaction before exit, else an unwanted error
		// may surface later (e.g. on database close).
		txns[action.TransactionID].Discard(ctx)
		return nil
	}

	return txns
}

// commitTransaction commits the given transaction.
//
// Will panic if the given transaction does not exist. Discards the transaction if
// an error is returned on commit.
func commitTransaction(
	ctx context.Context,
	t *testing.T,
	txns []datastore.Txn,
	testCase TestCase,
	action TransactionCommit,
) {
	err := txns[action.TransactionID].Commit(ctx)
	if err != nil {
		txns[action.TransactionID].Discard(ctx)
	}

	expectedErrorRaised := AssertError(t, testCase.Description, err, action.ExpectedError)

	assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
}

// executeRequest executes the given request.
func executeRequest(
	ctx context.Context,
	t *testing.T,
	nodes []*node.Node,
	testCase TestCase,
	action Request,
) {
	var expectedErrorRaised bool
	for nodeID, node := range getNodes(action.NodeID, nodes) {
		result := node.DB.ExecRequest(ctx, action.Request)

		anyOfByFieldKey := map[docFieldKey][]any{}
		expectedErrorRaised = assertRequestResults(
			ctx,
			t,
			testCase.Description,
			&result.GQL,
			action.Results,
			action.ExpectedError,
			nodeID,
			anyOfByFieldKey,
		)
	}

	assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
}

// executeSubscriptionRequest executes the given subscription request, returning
// a channel that will receive a single event once the subscription has been completed.
//
// The returned channel will receive a function that asserts that
// the subscription received all its expected results and no more.
// It should be called from the main test routine to ensure that
// failures are recorded properly. It will only yield once, once
// the subscription has terminated.
func executeSubscriptionRequest(
	ctx context.Context,
	t *testing.T,
	allActionsDone chan struct{},
	db client.DB,
	testCase TestCase,
	action SubscriptionRequest,
) (chan func(), bool) {
	subscriptionAssert := make(chan func())

	result := db.ExecRequest(ctx, action.Request)
	if AssertErrors(t, testCase.Description, result.GQL.Errors, action.ExpectedError) {
		return nil, true
	}

	go func() {
		data := []map[string]any{}
		errs := []error{}

		allActionsAreDone := false
		expectedDataRecieved := len(action.Results) == 0
		stream := result.Pub.Stream()
		for {
			select {
			case s := <-stream:
				sResult, _ := s.(client.GQLResult)
				sData, _ := sResult.Data.([]map[string]any)
				errs = append(errs, sResult.Errors...)
				data = append(data, sData...)

				if len(data) >= len(action.Results) {
					expectedDataRecieved = true
				}

			case <-allActionsDone:
				allActionsAreDone = true
			}

			if expectedDataRecieved && allActionsAreDone {
				finalResult := &client.GQLResult{
					Data:   data,
					Errors: errs,
				}

				subscriptionAssert <- func() {
					// This assert should be executed from the main test routine
					// so that failures will be properly handled.
					expectedErrorRaised := assertRequestResults(
						ctx,
						t,
						testCase.Description,
						finalResult,
						action.Results,
						action.ExpectedError,
						// anyof is not yet supported by subscription requests
						0,
						map[docFieldKey][]any{},
					)

					assertExpectedErrorRaised(t, testCase.Description, action.ExpectedError, expectedErrorRaised)
				}

				return
			}
		}
	}()

	return subscriptionAssert, false
}

// Asserts as to whether an error has been raised as expected (or not). If an expected
// error has been raised it will return true, returns false in all other cases.
func AssertError(t *testing.T, description string, err error, expectedError string) bool {
	if err == nil {
		return false
	}

	if expectedError == "" {
		require.NoError(t, err, description)
		return false
	} else {
		if !strings.Contains(err.Error(), expectedError) {
			assert.ErrorIs(t, err, errors.New(expectedError))
			return false
		}
		return true
	}
}

// Asserts as to whether an error has been raised as expected (or not). If an expected
// error has been raised it will return true, returns false in all other cases.
func AssertErrors(
	t *testing.T,
	description string,
	errs []error,
	expectedError string,
) bool {
	if expectedError == "" {
		require.Empty(t, errs, description)
	} else {
		for _, e := range errs {
			// This is always a string at the moment, add support for other types as and when needed
			errorString := e.Error()
			if !strings.Contains(errorString, expectedError) {
				// We use ErrorIs for clearer failures (is a error comparison even if it is just a string)
				assert.ErrorIs(t, errors.New(errorString), errors.New(expectedError))
				continue
			}
			return true
		}
	}
	return false
}

// docFieldKey is an internal key type that wraps docIndex and fieldName
type docFieldKey struct {
	docIndex  int
	fieldName string
}

func assertRequestResults(
	ctx context.Context,
	t *testing.T,
	description string,
	result *client.GQLResult,
	expectedResults []map[string]any,
	expectedError string,
	nodeID int,
	anyOfByField map[docFieldKey][]any,
) bool {
	if AssertErrors(t, description, result.Errors, expectedError) {
		return true
	}

	if expectedResults == nil && result.Data == nil {
		return true
	}

	// Note: if result.Data == nil this panics (the panic seems useful while testing).
	resultantData := result.Data.([]map[string]any)

	log.Info(ctx, "", logging.NewKV("RequestResults", result.Data))

	// compare results
	assert.Equal(t, len(expectedResults), len(resultantData), description)
	if len(expectedResults) == 0 {
		// Need `require` here otherwise will panic in the for loop that ranges over
		// resultantData and tries to access expectedResults[0].
		require.Equal(t, expectedResults, resultantData)
	}

	for docIndex, result := range resultantData {
		expectedResult := expectedResults[docIndex]
		for field, actualValue := range result {
			expectedValue := expectedResult[field]

			switch r := expectedValue.(type) {
			case AnyOf:
				assert.Contains(t, r, actualValue)

				dfk := docFieldKey{docIndex, field}
				valueSet := anyOfByField[dfk]
				valueSet = append(valueSet, actualValue)
				anyOfByField[dfk] = valueSet
			default:
				assert.Equal(t, expectedValue, actualValue, fmt.Sprintf("node: %v, doc: %v", nodeID, docIndex))
			}
		}
	}

	return false
}

func assertExpectedErrorRaised(t *testing.T, description string, expectedError string, wasRaised bool) {
	if expectedError != "" && !wasRaised {
		assert.Fail(t, "Expected an error however none was raised.", description)
	}
}

func assertIntrospectionResults(
	ctx context.Context,
	t *testing.T,
	description string,
	db client.DB,
	action IntrospectionRequest,
) bool {
	result := db.ExecRequest(ctx, action.Request)

	if AssertErrors(t, description, result.GQL.Errors, action.ExpectedError) {
		return true
	}
	resultantData := result.GQL.Data.(map[string]any)

	if len(action.ExpectedData) == 0 && len(action.ContainsData) == 0 {
		require.Equal(t, action.ExpectedData, resultantData)
	}

	if len(action.ExpectedData) == 0 && len(action.ContainsData) > 0 {
		assertContains(t, action.ContainsData, resultantData)
	} else {
		require.Equal(t, len(action.ExpectedData), len(resultantData))

		for k, result := range resultantData {
			assert.Equal(t, action.ExpectedData[k], result)
		}
	}

	return false
}

// Asserts that the client introspection results conform to our expectations.
func assertClientIntrospectionResults(
	ctx context.Context,
	t *testing.T,
	description string,
	db client.DB,
	action ClientIntrospectionRequest,
) bool {
	result := db.ExecRequest(ctx, action.Request)

	if AssertErrors(t, description, result.GQL.Errors, action.ExpectedError) {
		return true
	}
	resultantData := result.GQL.Data.(map[string]any)

	if len(resultantData) == 0 {
		return false
	}

	// Iterate through all types, validating each type definition.
	// Inspired from buildClientSchema.ts from graphql-js,
	// which is one way that clients do validate the schema.
	types := resultantData["__schema"].(map[string]any)["types"].([]any)

	for _, typeData := range types {
		typeDef := typeData.(map[string]any)
		kind := typeDef["kind"].(string)

		switch kind {
		case "SCALAR", "INTERFACE", "UNION", "ENUM":
			// No validation for these types in this test
		case "OBJECT":
			fields := typeDef["fields"]
			if fields == nil {
				t.Errorf("Fields are missing for OBJECT type %v", typeDef["name"])
			}
		case "INPUT_OBJECT":
			inputFields := typeDef["inputFields"]
			if inputFields == nil {
				t.Errorf("InputFields are missing for INPUT_OBJECT type %v", typeDef["name"])
			}
		default:
			// t.Errorf("Unknown type kind: %v", kind)
		}
	}

	return true
}

// Asserts that the `actual` contains the given `contains` value according to the logic
// described on the [RequestTestCase.ContainsData] property.
func assertContains(t *testing.T, contains map[string]any, actual map[string]any) {
	for k, expected := range contains {
		innerActual := actual[k]
		if innerExpected, innerIsMap := expected.(map[string]any); innerIsMap {
			if innerActual == nil {
				assert.Equal(t, innerExpected, innerActual)
			} else if innerActualMap, isMap := innerActual.(map[string]any); isMap {
				// If the inner is another map then we continue down the chain
				assertContains(t, innerExpected, innerActualMap)
			} else {
				// If the types don't match then we use assert.Equal for a clean failure message
				assert.Equal(t, innerExpected, innerActual)
			}
		} else if innerExpected, innerIsArray := expected.([]any); innerIsArray {
			if actualArray, isActualArray := innerActual.([]any); isActualArray {
				// If the inner is an array/slice, then assert that each expected item is present
				// in the actual.  Note how the actual may contain additional items - this should
				// not result in a test failure.
				for _, innerExpectedItem := range innerExpected {
					assert.Contains(t, actualArray, innerExpectedItem)
				}
			} else {
				// If the types don't match then we use assert.Equal for a clean failure message
				assert.Equal(t, expected, innerActual)
			}
		} else {
			assert.Equal(t, expected, innerActual)
		}
	}
}
