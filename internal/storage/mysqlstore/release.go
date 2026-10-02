package mysqlstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
)

const maxReleaseBytes = 5 << 20

type ReleasePrepare struct {
	OperationID                           string
	Actor                                 Actor
	EnvironmentKey, ConfigKey, ReleaseKey string
	Spec                                  machine.ReleaseSpec
}
type ReleaseActivate struct {
	OperationID                           string
	Actor                                 Actor
	EnvironmentKey, ConfigKey, ReleaseKey string
	ExpectedGeneration                    uint64
}
type ReleaseResult struct {
	Outcome Outcome `json:"outcome"`
	machine.ReleaseState
	Digest string `json:"digest,omitempty"`
}

func validateReleaseSpec(spec machine.ReleaseSpec) bool {
	if spec.ConfigRevision == 0 || !machine.ValidReleasePath(spec.ConfigPath) || len(spec.Files) > 31 {
		return false
	}
	paths := []string{spec.ConfigPath}
	for _, file := range spec.Files {
		if !validResourceKey(file.Namespace) || !validResourceKey(file.Item) || !validResourceKey(file.Field) || file.Revision == 0 || !machine.ValidReleasePath(file.Path) {
			return false
		}
		for _, existing := range paths {
			if file.Path == existing || strings.HasPrefix(file.Path, existing+"/") || strings.HasPrefix(existing, file.Path+"/") {
				return false
			}
		}
		paths = append(paths, file.Path)
	}
	return true
}

func (store *Store) PrepareRelease(ctx context.Context, request ReleasePrepare) (ReleaseResult, error) {
	if !validOperationID(request.OperationID) || !validScopedActor(request.Actor) || !validResourceKey(request.EnvironmentKey) || !validResourceKey(request.ConfigKey) || !validResourceKey(request.ReleaseKey) || !validateReleaseSpec(request.Spec) {
		return ReleaseResult{}, ErrValidation
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ReleaseResult{}, errors.New("begin release preparation")
	}
	defer tx.Rollback()
	if err := store.lockMasterKey(ctx, tx, false); err != nil {
		return ReleaseResult{}, err
	}
	token, err := authorizeScopedWrite(ctx, tx, request.Actor, request.EnvironmentKey, request.ConfigKey, "")
	if err != nil {
		return ReleaseResult{}, err
	}
	if request.Actor.Type == "token" {
		ctx = machine.WithToken(ctx, token)
	}
	for _, file := range request.Spec.Files {
		if request.Actor.Type == "token" && !token.AllowsNamespace(file.Namespace) {
			return ReleaseResult{}, machine.ErrForbidden
		}
	}
	replayed, replay, err := beginOperation(ctx, tx, request.OperationID, tokenRequestDigest(request), request.Actor)
	if err != nil {
		return ReleaseResult{}, err
	}
	if replayed {
		return replayRelease(replay)
	}
	finish := func(result ReleaseResult, failure error) (ReleaseResult, error) {
		return finishRelease(ctx, tx, request.OperationID, request.Actor, request.EnvironmentKey, request.ConfigKey, request.ReleaseKey, "prepare", result, failure)
	}
	envID, configID, current, err := releaseRoot(ctx, tx, request.EnvironmentKey, request.ConfigKey)
	if err != nil {
		return ReleaseResult{}, err
	}
	// Existing immutable keys can be retried after subsequent drafts changed.
	old, oldDigest, err := loadRelease(ctx, tx, envID, configID, request.ReleaseKey)
	if err == nil {
		left, _ := json.Marshal(old.ReleaseSpec)
		right, _ := json.Marshal(sortedReleaseSpec(request.Spec))
		if !bytes.Equal(left, right) {
			return finish(ReleaseResult{Outcome: OutcomeConflict}, ErrConflict)
		}
		if _, err := store.resolveRelease(ctx, tx, envID, configID, &old, false); err != nil {
			return ReleaseResult{}, err
		}
		return finish(ReleaseResult{Outcome: OutcomeNoChange, ReleaseState: machine.ReleaseState{ReleaseKey: request.ReleaseKey}, Digest: hex.EncodeToString(oldDigest)}, nil)
	}
	if !errors.Is(err, machine.ErrNotFound) {
		return ReleaseResult{}, err
	}
	if current != request.Spec.ConfigRevision {
		return finish(ReleaseResult{Outcome: OutcomeConflict}, ErrConflict)
	}
	manifest := machine.ReleaseManifest{Environment: request.EnvironmentKey, ConfigKey: request.ConfigKey, ReleaseKey: request.ReleaseKey, ReleaseSpec: sortedReleaseSpec(request.Spec)}
	if _, err := store.resolveRelease(ctx, tx, envID, configID, &manifest, true); err != nil {
		if errors.Is(err, ErrConflict) {
			return finish(ReleaseResult{Outcome: OutcomeConflict}, ErrConflict)
		}
		return ReleaseResult{}, err
	}
	encoded, _ := json.Marshal(manifest)
	digest := sha256.Sum256(encoded)
	inserted, err := tx.ExecContext(ctx, `INSERT INTO config_release_sets(environment_id,config_id,resource_key,manifest,digest) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE resource_key=resource_key`, envID, configID, request.ReleaseKey, string(encoded), digest[:])
	if err != nil {
		return ReleaseResult{}, errors.New("store immutable release")
	}
	rows, err := inserted.RowsAffected()
	if err != nil {
		return ReleaseResult{}, errors.New("inspect release preparation")
	}
	outcome := OutcomeSuccess
	if rows == 0 {
		// A locking read sees a concurrently committed immutable key, even when
		// the transaction's original consistent snapshot predates that key.
		var actual []byte
		if tx.QueryRowContext(ctx, `SELECT digest FROM config_release_sets WHERE environment_id=? AND config_id=? AND resource_key=? FOR SHARE`, envID, configID, request.ReleaseKey).Scan(&actual) != nil {
			return ReleaseResult{}, errors.New("read concurrent release")
		}
		if !bytes.Equal(actual, digest[:]) {
			return finish(ReleaseResult{Outcome: OutcomeConflict}, ErrConflict)
		}
		outcome = OutcomeNoChange
	}
	return finish(ReleaseResult{Outcome: outcome, ReleaseState: machine.ReleaseState{ReleaseKey: request.ReleaseKey}, Digest: hex.EncodeToString(digest[:])}, nil)
}

func sortedReleaseSpec(spec machine.ReleaseSpec) machine.ReleaseSpec {
	spec.Files = slices.Clone(spec.Files)
	if spec.Files == nil {
		spec.Files = []machine.ReleaseFileRef{}
	}
	slices.SortFunc(spec.Files, func(a, b machine.ReleaseFileRef) int { return strings.Compare(a.Path, b.Path) })
	return spec
}

func (store *Store) ActivateRelease(ctx context.Context, request ReleaseActivate) (ReleaseResult, error) {
	if !validOperationID(request.OperationID) || !validScopedActor(request.Actor) || !validResourceKey(request.EnvironmentKey) || !validResourceKey(request.ConfigKey) || !validResourceKey(request.ReleaseKey) {
		return ReleaseResult{}, ErrValidation
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ReleaseResult{}, errors.New("begin release activation")
	}
	defer tx.Rollback()
	if err := store.lockMasterKey(ctx, tx, false); err != nil {
		return ReleaseResult{}, err
	}
	token, err := authorizeScopedWrite(ctx, tx, request.Actor, request.EnvironmentKey, request.ConfigKey, "")
	if err != nil {
		return ReleaseResult{}, err
	}
	if request.Actor.Type == "token" {
		ctx = machine.WithToken(ctx, token)
	}
	replayed, replay, err := beginOperation(ctx, tx, request.OperationID, tokenRequestDigest(request), request.Actor)
	if err != nil {
		return ReleaseResult{}, err
	}
	if replayed {
		return replayRelease(replay)
	}
	envID, configID, _, err := releaseRoot(ctx, tx, request.EnvironmentKey, request.ConfigKey)
	if err != nil {
		return ReleaseResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO config_release_states(environment_id,config_id,generation) VALUES(?,?,0) ON DUPLICATE KEY UPDATE generation=generation`, envID, configID); err != nil {
		return ReleaseResult{}, errors.New("lock release stream")
	}
	var active sql.NullString
	var generation uint64
	if err := tx.QueryRowContext(ctx, `SELECT active_key,generation FROM config_release_states WHERE environment_id=? AND config_id=? FOR UPDATE`, envID, configID).Scan(&active, &generation); err != nil {
		return ReleaseResult{}, errors.New("read release stream")
	}
	finish := func(result ReleaseResult, failure error) (ReleaseResult, error) {
		return finishRelease(ctx, tx, request.OperationID, request.Actor, request.EnvironmentKey, request.ConfigKey, request.ReleaseKey, "activate", result, failure)
	}
	if generation != request.ExpectedGeneration {
		return finish(ReleaseResult{Outcome: OutcomeConflict, ReleaseState: machine.ReleaseState{ReleaseKey: active.String, Generation: generation}}, ErrConflict)
	}
	manifest, digest, err := loadRelease(ctx, tx, envID, configID, request.ReleaseKey)
	if err != nil {
		return ReleaseResult{}, err
	}
	if _, err := store.resolveRelease(ctx, tx, envID, configID, &manifest, false); err != nil {
		return ReleaseResult{}, err
	}
	result := ReleaseResult{Outcome: OutcomeNoChange, ReleaseState: machine.ReleaseState{ReleaseKey: request.ReleaseKey, Generation: generation}, Digest: hex.EncodeToString(digest)}
	if active.String != request.ReleaseKey {
		result.Outcome = OutcomeSuccess
		result.Generation++
		if _, err := tx.ExecContext(ctx, `UPDATE config_release_states SET active_key=?,generation=? WHERE environment_id=? AND config_id=?`, request.ReleaseKey, result.Generation, envID, configID); err != nil {
			return ReleaseResult{}, errors.New("activate release")
		}
	}
	return finish(result, nil)
}

func replayRelease(replay operationReplay) (ReleaseResult, error) {
	var result ReleaseResult
	if json.Unmarshal(replay.Response, &result) != nil || result.Outcome != replay.Outcome {
		return result, errors.New("invalid release replay")
	}
	return result, outcomeError(result.Outcome)
}
func finishRelease(ctx context.Context, tx *sql.Tx, operation string, actor Actor, environment, config, release, action string, result ReleaseResult, failure error) (ReleaseResult, error) {
	err := finishOperation(ctx, tx, operation, actor, result.Outcome, result.Generation, result, mutationEvent{Type: "release." + action, Action: "release." + action, EnvironmentKey: environment, ResourceType: "release", ResourceKey: config + "." + release}, action == "activate" && result.Outcome == OutcomeSuccess)
	if err != nil {
		return ReleaseResult{}, err
	}
	if tx.Commit() != nil {
		return ReleaseResult{}, errors.New("commit release operation")
	}
	return result, withCommittedAudit(failure)
}

func releaseRoot(ctx context.Context, tx *sql.Tx, environment, config string) ([]byte, []byte, uint64, error) {
	if token, ok := machine.TokenFromContext(ctx); ok && !token.AllowsConfig(config) {
		return nil, nil, 0, machine.ErrForbidden
	}
	var envID, configID []byte
	var revision uint64
	err := tx.QueryRowContext(ctx, `SELECT e.id,c.id,s.current_revision FROM environments e JOIN config_env_states s ON s.environment_id=e.id JOIN configs c ON c.id=s.config_id WHERE e.resource_key=? AND e.archived_at IS NULL AND c.resource_key=? AND c.archived_at IS NULL`, environment, config).Scan(&envID, &configID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, 0, machine.ErrNotFound
	}
	if err != nil {
		return nil, nil, 0, errors.New("read release anchor")
	}
	return envID, configID, revision, nil
}

func loadRelease(ctx context.Context, tx *sql.Tx, envID, configID []byte, key string) (machine.ReleaseManifest, []byte, error) {
	var encoded, digest []byte
	var manifest machine.ReleaseManifest
	err := tx.QueryRowContext(ctx, `SELECT manifest,digest FROM config_release_sets WHERE environment_id=? AND config_id=? AND resource_key=?`, envID, configID, key).Scan(&encoded, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return manifest, nil, machine.ErrNotFound
	}
	if err != nil {
		return manifest, nil, errors.New("read release manifest")
	}
	if len(encoded) > 128<<10 || json.Unmarshal(encoded, &manifest) != nil || !validateReleaseSpec(manifest.ReleaseSpec) || !validResourceKey(manifest.Environment) || !validResourceKey(manifest.ConfigKey) || manifest.ReleaseKey != key {
		return manifest, nil, machine.ErrIntegrity
	}
	canonical, _ := json.Marshal(manifest)
	hash := sha256.Sum256(canonical)
	if !bytes.Equal(hash[:], digest) {
		return manifest, nil, machine.ErrIntegrity
	}
	return manifest, digest, nil
}

func (store *Store) ReadReleaseState(ctx context.Context, environment, config string) (machine.ReleaseState, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return machine.ReleaseState{}, errors.New("begin release state read")
	}
	defer tx.Rollback()
	envID, configID, _, err := releaseRoot(ctx, tx, environment, config)
	if err != nil {
		return machine.ReleaseState{}, err
	}
	state, err := readReleaseState(ctx, tx, envID, configID)
	if err != nil {
		return state, err
	}
	if tx.Commit() != nil {
		return state, errors.New("complete release state read")
	}
	return state, nil
}
func readReleaseState(ctx context.Context, tx *sql.Tx, envID, configID []byte) (machine.ReleaseState, error) {
	var state machine.ReleaseState
	var key sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT active_key,generation FROM config_release_states WHERE environment_id=? AND config_id=?`, envID, configID).Scan(&key, &state.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, errors.New("read release state")
	}
	state.ReleaseKey = key.String
	return state, nil
}

func (store *Store) ReadRelease(ctx context.Context, environment, config, key, ifNoneMatch string) (machine.ReleaseBundle, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return machine.ReleaseBundle{}, errors.New("begin release snapshot")
	}
	defer tx.Rollback()
	envID, configID, _, err := releaseRoot(ctx, tx, environment, config)
	if err != nil {
		return machine.ReleaseBundle{}, err
	}
	var generation uint64
	if key == "" {
		state, err := readReleaseState(ctx, tx, envID, configID)
		if err != nil {
			return machine.ReleaseBundle{}, err
		}
		key, generation = state.ReleaseKey, state.Generation
		if key == "" {
			return machine.ReleaseBundle{}, machine.ErrNotFound
		}
	}
	manifest, digest, err := loadRelease(ctx, tx, envID, configID, key)
	if err != nil {
		return machine.ReleaseBundle{}, err
	}
	if manifest.Environment != environment || manifest.ConfigKey != config {
		return machine.ReleaseBundle{}, machine.ErrIntegrity
	}
	result, err := store.resolveRelease(ctx, tx, envID, configID, &manifest, false)
	if err != nil {
		return machine.ReleaseBundle{}, err
	}
	result.Generation = generation
	result.Digest = hex.EncodeToString(digest)
	result.ETag = fmt.Sprintf(`"release-%s-%d"`, result.Digest, generation)
	if tx.Commit() != nil {
		return machine.ReleaseBundle{}, errors.New("complete release snapshot")
	}
	if ifNoneMatch == result.ETag {
		return machine.ReleaseBundle{ETag: result.ETag}, machine.ErrNotModified
	}
	return result, nil
}

// One transaction resolves the complete batch. Historical revisions never
// bypass current item availability, Environment bindings, or active Fields.
func (store *Store) resolveRelease(ctx context.Context, tx *sql.Tx, envID, configID []byte, manifest *machine.ReleaseManifest, prepare bool) (machine.ReleaseBundle, error) {
	var format string
	var canonical []byte
	if err := tx.QueryRowContext(ctx, `SELECT format,content FROM config_revisions WHERE environment_id=? AND config_id=? AND revision=?`, envID, configID, manifest.ConfigRevision).Scan(&format, &canonical); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return machine.ReleaseBundle{}, machine.ErrNotFound
		}
		return machine.ReleaseBundle{}, errors.New("read pinned Config")
	}
	if len(canonical) > maxReleaseBytes {
		return machine.ReleaseBundle{}, machine.ErrIntegrity
	}
	references, err := readReferences(ctx, tx, configID, envID, manifest.ConfigRevision)
	if err != nil {
		return machine.ReleaseBundle{}, err
	}
	needed := make(map[vaultItemIdentity][]string, len(references))
	fields := 0
	for identity, keys := range references {
		needed[identity] = slices.Clone(keys)
		fields += len(keys)
	}
	for _, file := range manifest.Files {
		identity := vaultItemIdentity{file.Namespace, file.Item}
		needed[identity] = append(needed[identity], file.Field)
		fields++
	}
	if len(needed) > 64 || fields > 256 {
		return machine.ReleaseBundle{}, ErrValidation
	}
	if prepare {
		manifest.VaultRevisions = map[string]uint64{}
	} else if len(manifest.VaultRevisions) != len(needed) {
		return machine.ReleaseBundle{}, machine.ErrIntegrity
	}
	values := map[configdoc.Reference]string{}
	files := make(map[string][]byte, len(manifest.Files))
	size := 0
	textSize := 0
	for identity, keys := range needed {
		if token, ok := machine.TokenFromContext(ctx); ok && !token.AllowsNamespace(identity.namespaceKey) {
			return machine.ReleaseBundle{}, machine.ErrForbidden
		}
		current, err := readOneVaultMetadata(ctx, tx, envID, identity)
		if err != nil {
			return machine.ReleaseBundle{}, err
		}
		currentTypes, err := readFieldTypes(ctx, tx, current, keys)
		if err != nil {
			return machine.ReleaseBundle{}, err
		}
		if prepare {
			manifest.VaultRevisions[identity.revisionKey()] = current.revision
		}
		pinned := current
		pinned.revision = manifest.VaultRevisions[identity.revisionKey()]
		if pinned.revision == 0 {
			return machine.ReleaseBundle{}, machine.ErrIntegrity
		}
		for _, file := range manifest.Files {
			if file.Namespace == identity.namespaceKey && file.Item == identity.itemKey && file.Revision != pinned.revision {
				if prepare {
					return machine.ReleaseBundle{}, ErrConflict
				}
				return machine.ReleaseBundle{}, machine.ErrIntegrity
			}
		}
		if err := tx.QueryRowContext(ctx, `SELECT variant_id FROM vault_revision_variant_environments WHERE item_id=? AND revision=? AND environment_id=?`, pinned.itemID, pinned.revision, envID).Scan(&pinned.variantID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return machine.ReleaseBundle{}, machine.ErrUnresolved
			}
			return machine.ReleaseBundle{}, errors.New("read pinned binding")
		}
		types, err := readFieldTypes(ctx, tx, pinned, keys)
		if err != nil {
			return machine.ReleaseBundle{}, err
		}
		payload, err := store.decryptVaultPayload(ctx, tx, pinned)
		if err != nil {
			return machine.ReleaseBundle{}, err
		}
		variant, ok := payload.Variants[hex.EncodeToString(pinned.variantID)]
		if !ok {
			return machine.ReleaseBundle{}, machine.ErrIntegrity
		}
		for _, field := range references[identity] {
			value, ok := variant.Values[field]
			if !ok || value.Text == nil || (types[field] != "text" && types[field] != "secret") || (currentTypes[field] != "text" && currentTypes[field] != "secret") {
				return machine.ReleaseBundle{}, machine.ErrUnresolved
			}
			textSize += len(*value.Text)
			if textSize > maxReleaseBytes {
				return machine.ReleaseBundle{}, ErrValidation
			}
			values[configdoc.Reference{NamespaceKey: identity.namespaceKey, ItemKey: identity.itemKey, FieldKey: field}] = *value.Text
		}
		for _, file := range manifest.Files {
			if file.Namespace != identity.namespaceKey || file.Item != identity.itemKey {
				continue
			}
			value, ok := variant.Values[file.Field]
			if !ok || value.File == nil || types[file.Field] != "file" || currentTypes[file.Field] != "file" {
				return machine.ReleaseBundle{}, machine.ErrUnresolved
			}
			size += len(value.File.Bytes)
			if size > maxReleaseBytes {
				return machine.ReleaseBundle{}, ErrValidation
			}
			files[file.Path] = bytes.Clone(value.File.Bytes)
		}
	}
	resolved, err := configdoc.Resolve(configdoc.Format(format), canonical, func(ref configdoc.Reference) (string, error) {
		value, ok := values[ref]
		if !ok {
			return "", machine.ErrUnresolved
		}
		return value, nil
	})
	if err != nil {
		return machine.ReleaseBundle{}, machine.ErrUnresolved
	}
	defer clear(resolved)
	if size+len(resolved) > maxReleaseBytes {
		return machine.ReleaseBundle{}, ErrValidation
	}
	result := machine.ReleaseBundle{Manifest: *manifest, Config: machine.ResolvedConfig{Format: format, Content: string(resolved), ConfigRevision: manifest.ConfigRevision, VaultRevisions: manifest.VaultRevisions}, Files: make([]machine.ReleaseFile, 0, len(files))}
	for _, file := range manifest.Files {
		result.Files = append(result.Files, machine.ReleaseFile{Path: file.Path, Bytes: files[file.Path]})
	}
	return result, nil
}
