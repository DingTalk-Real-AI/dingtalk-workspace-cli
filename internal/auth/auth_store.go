// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const (
	authRegistryVersion       = 1
	authRegistryDirName       = "auth"
	authRegistryFileName      = "registry.json"
	authRegistryLockFileName  = ".registry.lock"
	authInstancesDirName      = "instances"
	authInstanceServicePrefix = keychain.Service + "-instance-"
	DefaultAuthInstanceAlias  = "default"
	maxAuthInstanceAliasRunes = 64
	maxAuthRegistryFileBytes  = 1 << 20
)

// AuthInstanceKind identifies whether an instance reuses the historical
// online store or owns an explicitly-created isolated store.
type AuthInstanceKind string

const (
	AuthInstanceKindLegacy   AuthInstanceKind = "legacy"
	AuthInstanceKindIsolated AuthInstanceKind = "isolated"
)

var (
	// ErrAuthInstanceIsolationUnsupported is returned before an isolated state
	// is created when an edition owns token persistence through hooks. Splitting
	// only part of a hook-owned backend would make instance selection ambiguous.
	ErrAuthInstanceIsolationUnsupported = errors.New("isolated auth instances are not supported by the current edition token hooks")
	ErrAuthRegistryConflict             = errors.New("auth instance registry changed; retry the operation")
)

// AuthInstance is non-sensitive metadata for one login-state storage
// location. It does not bind a natural person or organization. Identities
// remain in the instance-local profiles.json.
type AuthInstance struct {
	ID    string           `json:"id"`
	Alias string           `json:"alias"`
	Kind  AuthInstanceKind `json:"kind"`
}

// AuthStore is the effective user-auth storage coordinate for one process.
// BaseConfigDir continues to own shared application configuration. ConfigDir
// and KeychainService contain only the selected login state.
type AuthStore struct {
	BaseConfigDir   string
	ConfigDir       string
	KeychainService string
	InstanceID      string
	InstanceAlias   string
	InstanceKind    AuthInstanceKind
}

// Isolated reports whether this store belongs to an explicitly-created auth
// instance. The legacy-backed default returns false.
func (s AuthStore) Isolated() bool {
	return s.InstanceKind == AuthInstanceKindIsolated
}

type authRegistry struct {
	Version            int            `json:"version"`
	Revision           int64          `json:"revision"`
	CurrentInstanceID  string         `json:"currentInstanceId"`
	PreviousInstanceID string         `json:"previousInstanceId,omitempty"`
	Instances          []AuthInstance `json:"instances"`
}

var (
	runtimeAuthStores sync.Map // normalized base config dir -> AuthStore
	registryLocks     sync.Map // normalized base config dir -> *sync.Mutex
	pendingAuthTxs    sync.Map // isolated keychain service -> *NewInstanceTransaction
)

// ResolveAuthStore is deliberately side-effect free. Unless an instance was
// explicitly activated in this process, it returns the exact historical
// configDir + dws-cli coordinate, including custom DWS_CONFIG_DIR values.
func ResolveAuthStore(baseConfigDir string) AuthStore {
	if value, ok := runtimeAuthStores.Load(authStoreRuntimeKey(baseConfigDir)); ok {
		if store, valid := value.(AuthStore); valid {
			return store
		}
	}
	return legacyAuthStore(baseConfigDir, AuthInstance{})
}

// DeactivateAuthStore clears only the process-local selection. It never
// changes registry.json or deletes credentials.
func DeactivateAuthStore(baseConfigDir string) {
	runtimeAuthStores.Delete(authStoreRuntimeKey(baseConfigDir))
}

// AuthRegistryPath returns the fixed registry location for one DWS config
// home. Merely resolving this path does not create it.
func AuthRegistryPath(baseConfigDir string) string {
	return filepath.Join(baseConfigDir, authRegistryDirName, authRegistryFileName)
}

// ActivateCurrentInstance activates the persisted current auth instance. A
// missing registry is the online-compatible state: it performs no write and
// keeps the historical configDir + dws-cli coordinate. A malformed registry
// fails closed instead of falling back to unrelated legacy credentials.
func ActivateCurrentInstance(baseConfigDir string) (AuthStore, error) {
	registry, exists, err := readAuthRegistry(baseConfigDir)
	if err != nil {
		DeactivateAuthStore(baseConfigDir)
		return AuthStore{}, err
	}
	if !exists {
		DeactivateAuthStore(baseConfigDir)
		return ResolveAuthStore(baseConfigDir), nil
	}
	instance := findAuthInstanceByID(&registry, registry.CurrentInstanceID)
	if instance == nil {
		DeactivateAuthStore(baseConfigDir)
		return AuthStore{}, fmt.Errorf("invalid auth registry: current instance %q not found", registry.CurrentInstanceID)
	}
	if authHooksOwnTokenStorage() && instance.Kind == AuthInstanceKindIsolated {
		DeactivateAuthStore(baseConfigDir)
		return AuthStore{}, fmt.Errorf(
			"%w: selected instance %q is isolated; switch it with an edition that uses the standard DWS auth store, or use a separate DWS_CONFIG_DIR",
			ErrAuthInstanceIsolationUnsupported, instance.Alias,
		)
	}
	store := authStoreForInstance(baseConfigDir, *instance)
	setRuntimeAuthStore(baseConfigDir, store)
	return store, nil
}

// ListAuthInstances returns registry order and the current ID. Before explicit
// opt-in it reports one synthetic legacy instance without creating a file.
func ListAuthInstances(baseConfigDir string) (instances []AuthInstance, currentID string, err error) {
	registry, exists, err := readAuthRegistry(baseConfigDir)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		return []AuthInstance{{Alias: DefaultAuthInstanceAlias, Kind: AuthInstanceKindLegacy}}, "", nil
	}
	return append([]AuthInstance(nil), registry.Instances...), registry.CurrentInstanceID, nil
}

// UseAuthInstance makes one login-state storage location current. Selector
// accepts its alias, generated ID, or "-" for the previous instance.
func UseAuthInstance(baseConfigDir, selector string) (AuthInstance, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return AuthInstance{}, fmt.Errorf("auth instance selector is required")
	}

	var selected AuthInstance
	var selectedStore AuthStore
	err := withAuthRegistryLock(baseConfigDir, func() error {
		registry, exists, err := readAuthRegistry(baseConfigDir)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("isolated auth instances are not enabled; use auth login --new-instance <alias>")
		}
		lookup := selector
		if selector == "-" {
			lookup = registry.PreviousInstanceID
			if lookup == "" {
				return fmt.Errorf("previous auth instance is empty")
			}
		}
		instance := findAuthInstance(&registry, lookup)
		if instance == nil {
			return fmt.Errorf("auth instance %q not found", selector)
		}
		if authHooksOwnTokenStorage() && instance.Kind == AuthInstanceKindIsolated {
			return fmt.Errorf("%w: instance %q is isolated", ErrAuthInstanceIsolationUnsupported, instance.Alias)
		}
		selected = *instance
		selectedStore = authStoreForInstance(baseConfigDir, selected)
		if selected.ID == registry.CurrentInstanceID {
			setRuntimeAuthStore(baseConfigDir, selectedStore)
			return nil
		}
		registry.PreviousInstanceID = registry.CurrentInstanceID
		registry.CurrentInstanceID = selected.ID
		registry.Revision++
		if err := writeAuthRegistry(baseConfigDir, registry); err != nil {
			return err
		}
		setRuntimeAuthStore(baseConfigDir, selectedStore)
		return nil
	})
	if err != nil {
		return AuthInstance{}, err
	}
	return selected, nil
}

// NewInstanceTransaction keeps a new instance invisible to other processes
// until the existing login flow succeeds and Commit atomically publishes it.
type NewInstanceTransaction struct {
	mu                      sync.Mutex
	baseConfigDir           string
	expectedRegistryExists  bool
	expectedRevision        int64
	pending                 authRegistry
	instance                AuthInstance
	store                   AuthStore
	previousStore           AuthStore
	previousWasExplicit     bool
	writtenKeychainAccounts map[string]struct{}
	done                    bool
}

// BeginNewInstance process-activates a fresh isolated store without copying
// any legacy token, profile, marker, secret, or application config. Commit must
// be called only after login succeeds; otherwise call Rollback.
func BeginNewInstance(baseConfigDir, alias string) (*NewInstanceTransaction, error) {
	if strings.TrimSpace(baseConfigDir) == "" {
		return nil, fmt.Errorf("base config directory is required")
	}
	if authHooksOwnTokenStorage() {
		return nil, ErrAuthInstanceIsolationUnsupported
	}
	alias, err := validateAuthInstanceAlias(alias)
	if err != nil {
		return nil, err
	}

	registry, exists, err := readAuthRegistry(baseConfigDir)
	if err != nil {
		return nil, err
	}
	previousStore := ResolveAuthStore(baseConfigDir)
	previousWasExplicit := previousStore.InstanceID != ""
	if exists {
		if findAuthInstance(&registry, alias) != nil {
			return nil, fmt.Errorf("auth instance alias %q already exists", alias)
		}
		if previousStore.InstanceID == "" {
			current := findAuthInstanceByID(&registry, registry.CurrentInstanceID)
			if current == nil {
				return nil, fmt.Errorf("invalid auth registry: current instance %q not found", registry.CurrentInstanceID)
			}
			previousStore = authStoreForInstance(baseConfigDir, *current)
			previousWasExplicit = true
		}
	} else {
		if strings.EqualFold(alias, DefaultAuthInstanceAlias) {
			return nil, fmt.Errorf("auth instance alias %q is reserved for the existing login", alias)
		}
		legacyInstance := AuthInstance{
			ID:    uuid.NewString(),
			Alias: DefaultAuthInstanceAlias,
			Kind:  AuthInstanceKindLegacy,
		}
		registry = authRegistry{
			Version:           authRegistryVersion,
			CurrentInstanceID: legacyInstance.ID,
			Instances:         []AuthInstance{legacyInstance},
		}
	}

	instance := AuthInstance{ID: uuid.NewString(), Alias: alias, Kind: AuthInstanceKindIsolated}
	registry.PreviousInstanceID = registry.CurrentInstanceID
	registry.CurrentInstanceID = instance.ID
	registry.Instances = append(registry.Instances, instance)
	registry.Revision++
	if err := validateAuthRegistry(&registry); err != nil {
		return nil, err
	}
	store := authStoreForInstance(baseConfigDir, instance)
	tx := &NewInstanceTransaction{
		baseConfigDir:           baseConfigDir,
		expectedRegistryExists:  exists,
		expectedRevision:        registry.Revision - 1,
		pending:                 registry,
		instance:                instance,
		store:                   store,
		previousStore:           previousStore,
		previousWasExplicit:     previousWasExplicit,
		writtenKeychainAccounts: make(map[string]struct{}),
	}
	pendingAuthTxs.Store(store.KeychainService, tx)
	setRuntimeAuthStore(baseConfigDir, store)
	return tx, nil
}

// Store returns the pending isolated storage coordinate used by login.
func (tx *NewInstanceTransaction) Store() AuthStore {
	if tx == nil {
		return AuthStore{}
	}
	return tx.store
}

// Instance returns the pending instance metadata.
func (tx *NewInstanceTransaction) Instance() AuthInstance {
	if tx == nil {
		return AuthInstance{}
	}
	return tx.instance
}

// Commit atomically publishes the pending instance after verifying that no
// other process changed the registry during interactive login.
func (tx *NewInstanceTransaction) Commit() error {
	if tx == nil {
		return fmt.Errorf("nil auth instance transaction")
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return fmt.Errorf("auth instance transaction is already closed")
	}

	err := withAuthRegistryLock(tx.baseConfigDir, func() error {
		current, exists, err := readAuthRegistry(tx.baseConfigDir)
		if err != nil {
			return err
		}
		if exists != tx.expectedRegistryExists {
			return ErrAuthRegistryConflict
		}
		if exists && current.Revision != tx.expectedRevision {
			return ErrAuthRegistryConflict
		}
		if err := ensurePrivateAuthStoreDirs(tx.store); err != nil {
			return err
		}
		return writeAuthRegistry(tx.baseConfigDir, tx.pending)
	})
	if err != nil {
		return err
	}
	tx.done = true
	pendingAuthTxs.Delete(tx.store.KeychainService)
	setRuntimeAuthStore(tx.baseConfigDir, tx.store)
	return nil
}

// Rollback restores the previous process selection and best-effort removes the
// unpublished instance's unique files and token slots. It never writes the
// registry.
func (tx *NewInstanceTransaction) Rollback() error {
	if tx == nil {
		return nil
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return nil
	}
	tx.done = true
	pendingAuthTxs.Delete(tx.store.KeychainService)
	cleanupErr := cleanupUncommittedAuthStore(tx.store, tx.writtenKeychainAccounts)
	if tx.previousWasExplicit {
		setRuntimeAuthStore(tx.baseConfigDir, tx.previousStore)
	} else {
		DeactivateAuthStore(tx.baseConfigDir)
	}
	return cleanupErr
}

func legacyAuthStore(baseConfigDir string, instance AuthInstance) AuthStore {
	return AuthStore{
		BaseConfigDir:   baseConfigDir,
		ConfigDir:       baseConfigDir,
		KeychainService: keychain.Service,
		InstanceID:      instance.ID,
		InstanceAlias:   instance.Alias,
		InstanceKind:    instance.Kind,
	}
}

func authStoreForInstance(baseConfigDir string, instance AuthInstance) AuthStore {
	if instance.Kind == AuthInstanceKindLegacy {
		return legacyAuthStore(baseConfigDir, instance)
	}
	return AuthStore{
		BaseConfigDir:   baseConfigDir,
		ConfigDir:       filepath.Join(baseConfigDir, authRegistryDirName, authInstancesDirName, instance.ID),
		KeychainService: authInstanceServicePrefix + instance.ID,
		InstanceID:      instance.ID,
		InstanceAlias:   instance.Alias,
		InstanceKind:    instance.Kind,
	}
}

func setRuntimeAuthStore(baseConfigDir string, store AuthStore) {
	runtimeAuthStores.Store(authStoreRuntimeKey(baseConfigDir), store)
}

func authStoreRuntimeKey(baseConfigDir string) string {
	clean := filepath.Clean(baseConfigDir)
	if absolute, err := filepath.Abs(clean); err == nil {
		clean = absolute
	}
	return clean
}

func authRegistryRoot(baseConfigDir string) string {
	return filepath.Join(baseConfigDir, authRegistryDirName)
}

func readAuthRegistry(baseConfigDir string) (authRegistry, bool, error) {
	path := AuthRegistryPath(baseConfigDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return authRegistry{}, false, nil
		}
		return authRegistry{}, false, fmt.Errorf("read auth registry: %w", err)
	}
	if len(data) > maxAuthRegistryFileBytes {
		return authRegistry{}, true, fmt.Errorf("auth registry exceeds %d bytes", maxAuthRegistryFileBytes)
	}
	var registry authRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return authRegistry{}, true, fmt.Errorf("parse auth registry: %w", err)
	}
	if err := validateAuthRegistry(&registry); err != nil {
		return authRegistry{}, true, err
	}
	return registry, true, nil
}

func writeAuthRegistry(baseConfigDir string, registry authRegistry) error {
	if err := validateAuthRegistry(&registry); err != nil {
		return err
	}
	data, err := json.MarshalIndent(&registry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth registry: %w", err)
	}
	if err := ensurePrivateDir(authRegistryRoot(baseConfigDir)); err != nil {
		return err
	}
	if err := helpers.AtomicWrite(AuthRegistryPath(baseConfigDir), append(data, '\n'), config.FilePerm); err != nil {
		return fmt.Errorf("write auth registry: %w", err)
	}
	return nil
}

func validateAuthRegistry(registry *authRegistry) error {
	if registry == nil {
		return fmt.Errorf("invalid auth registry: empty")
	}
	if registry.Version != authRegistryVersion {
		return fmt.Errorf("unsupported auth registry version %d", registry.Version)
	}
	if registry.Revision < 1 {
		return fmt.Errorf("invalid auth registry revision %d", registry.Revision)
	}
	if len(registry.Instances) == 0 {
		return fmt.Errorf("invalid auth registry: instances are empty")
	}
	ids := make(map[string]struct{}, len(registry.Instances))
	aliases := make([]string, 0, len(registry.Instances))
	legacyCount := 0
	for i := range registry.Instances {
		instance := &registry.Instances[i]
		if err := validateCanonicalUUID("instance", instance.ID); err != nil {
			return err
		}
		if _, duplicate := ids[instance.ID]; duplicate {
			return fmt.Errorf("invalid auth registry: duplicate instance id %q", instance.ID)
		}
		ids[instance.ID] = struct{}{}
		alias, err := validateAuthInstanceAlias(instance.Alias)
		if err != nil {
			return fmt.Errorf("invalid auth registry instance %q: %w", instance.ID, err)
		}
		if alias != instance.Alias {
			return fmt.Errorf("invalid auth registry instance %q: alias is not normalized", instance.ID)
		}
		for _, existing := range aliases {
			if strings.EqualFold(existing, alias) {
				return fmt.Errorf("invalid auth registry: duplicate instance alias %q", alias)
			}
		}
		aliases = append(aliases, alias)
		switch instance.Kind {
		case AuthInstanceKindLegacy:
			legacyCount++
		case AuthInstanceKindIsolated:
		default:
			return fmt.Errorf("invalid auth registry instance %q: unsupported kind %q", instance.ID, instance.Kind)
		}
	}
	if legacyCount != 1 {
		return fmt.Errorf("invalid auth registry: expected one legacy instance, got %d", legacyCount)
	}
	if _, ok := ids[registry.CurrentInstanceID]; !ok {
		return fmt.Errorf("invalid auth registry: current instance %q not found", registry.CurrentInstanceID)
	}
	if previous := registry.PreviousInstanceID; previous != "" {
		if _, ok := ids[previous]; !ok {
			return fmt.Errorf("invalid auth registry: previous instance %q not found", previous)
		}
		if previous == registry.CurrentInstanceID {
			return fmt.Errorf("invalid auth registry: previous and current instances are identical")
		}
	}
	return nil
}

func validateCanonicalUUID(kind, value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value {
		return fmt.Errorf("invalid auth registry %s id %q", kind, value)
	}
	return nil
}

func validateAuthInstanceAlias(alias string) (string, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "", fmt.Errorf("auth instance alias is required")
	}
	if !utf8.ValidString(alias) {
		return "", fmt.Errorf("auth instance alias must be valid UTF-8")
	}
	if utf8.RuneCountInString(alias) > maxAuthInstanceAliasRunes {
		return "", fmt.Errorf("auth instance alias exceeds %d characters", maxAuthInstanceAliasRunes)
	}
	for _, r := range alias {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("auth instance alias contains control characters")
		}
	}
	if alias == "-" {
		return "", fmt.Errorf("auth instance alias %q is reserved", alias)
	}
	return alias, nil
}

func findAuthInstance(registry *authRegistry, selector string) *AuthInstance {
	if registry == nil {
		return nil
	}
	selector = strings.TrimSpace(selector)
	for i := range registry.Instances {
		instance := &registry.Instances[i]
		if instance.ID == selector || strings.EqualFold(instance.Alias, selector) {
			return instance
		}
	}
	return nil
}

func findAuthInstanceByID(registry *authRegistry, id string) *AuthInstance {
	if registry == nil {
		return nil
	}
	for i := range registry.Instances {
		if registry.Instances[i].ID == id {
			return &registry.Instances[i]
		}
	}
	return nil
}

func authHooksOwnTokenStorage() bool {
	hooks := edition.Get()
	return hooks != nil && (hooks.SaveToken != nil || hooks.LoadToken != nil || hooks.DeleteToken != nil)
}

func withAuthRegistryLock(baseConfigDir string, fn func() error) error {
	key := authStoreRuntimeKey(baseConfigDir)
	value, _ := registryLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	root := authRegistryRoot(baseConfigDir)
	if err := ensurePrivateDir(root); err != nil {
		return err
	}
	path := filepath.Join(root, authRegistryLockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, config.FilePerm)
	if err != nil {
		return fmt.Errorf("open auth registry lock: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(config.FilePerm); err != nil {
		return fmt.Errorf("set auth registry lock permissions: %w", err)
	}
	deadline := time.Now().Add(config.LockTimeout)
	for {
		if err := lockFile(file); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout acquiring auth registry lock after %v", config.LockTimeout)
		}
		time.Sleep(lockRetryDelay)
	}
	defer unlockFile(file)
	return fn()
}

func ensurePrivateAuthStoreDirs(store AuthStore) error {
	if !store.Isolated() {
		return nil
	}
	return ensurePrivateDir(store.ConfigDir)
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, config.DirPerm); err != nil {
		return fmt.Errorf("create private auth directory: %w", err)
	}
	if err := os.Chmod(path, config.DirPerm); err != nil {
		return fmt.Errorf("set private auth directory permissions: %w", err)
	}
	return nil
}

func recordPendingAuthKeychainWrite(service, account string) {
	value, ok := pendingAuthTxs.Load(service)
	if !ok {
		return
	}
	tx, ok := value.(*NewInstanceTransaction)
	if !ok || tx == nil {
		return
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return
	}
	tx.writtenKeychainAccounts[account] = struct{}{}
}

func cleanupUncommittedAuthStore(store AuthStore, writtenAccounts map[string]struct{}) error {
	if !store.Isolated() {
		return nil
	}
	var firstErr error
	accounts := make(map[string]struct{}, len(writtenAccounts)+1)
	for account := range writtenAccounts {
		accounts[account] = struct{}{}
	}
	accounts[keychain.AccountToken] = struct{}{}
	profilesPath := filepath.Join(store.ConfigDir, profilesJSONFile)
	if data, err := os.ReadFile(profilesPath); err == nil {
		var profiles ProfilesConfig
		if json.Unmarshal(data, &profiles) == nil {
			for _, profile := range profiles.Profiles {
				if profile.CorpID != "" {
					accounts[TokenAccountForCorpID(profile.CorpID)] = struct{}{}
				}
			}
		}
	}
	for account := range accounts {
		if err := keychain.Remove(store.KeychainService, account); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := os.RemoveAll(keychain.StorageDir(store.KeychainService)); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := os.RemoveAll(store.ConfigDir); err != nil && firstErr == nil {
		firstErr = err
	}
	pruneEmptyAuthParents(store)
	return firstErr
}

func pruneEmptyAuthParents(store AuthStore) {
	stop := authRegistryRoot(store.BaseConfigDir)
	for dir := filepath.Dir(store.ConfigDir); dir != stop && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			return
		}
	}
	_ = os.Remove(stop)
}
