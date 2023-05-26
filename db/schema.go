// Copyright 2022 Democratized Data Foundation
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package db

import (
	"context"
	"encoding/json"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/sourcenetwork/defradb/client"
	"github.com/sourcenetwork/defradb/datastore"
)

// addSchema takes the provided schema in SDL format, and applies it to the database,
// and creates the necessary collections, request types, etc.
func (db *db) addSchema(
	ctx context.Context,
	txn datastore.Txn,
	schemaString string,
) ([]client.CollectionDescription, error) {
	existingDescriptions, err := db.getCollectionDescriptions(ctx, txn)
	if err != nil {
		return nil, err
	}

	newDescriptions, err := db.parser.ParseSDL(ctx, schemaString)
	if err != nil {
		return nil, err
	}

	err = db.parser.SetSchema(ctx, txn, append(existingDescriptions, newDescriptions...))
	if err != nil {
		return nil, err
	}

	returnDescriptions := make([]client.CollectionDescription, len(newDescriptions))
	for i, desc := range newDescriptions {
		col, err := db.createCollection(ctx, txn, desc)
		if err != nil {
			return nil, err
		}
		returnDescriptions[i] = col.Description()
	}

	return returnDescriptions, nil
}

func (db *db) loadSchema(ctx context.Context, txn datastore.Txn) error {
	descriptions, err := db.getCollectionDescriptions(ctx, txn)
	if err != nil {
		return err
	}

	return db.parser.SetSchema(ctx, txn, descriptions)
}

func (db *db) getCollectionDescriptions(
	ctx context.Context,
	txn datastore.Txn,
) ([]client.CollectionDescription, error) {
	collections, err := db.getAllCollections(ctx, txn)
	if err != nil {
		return nil, err
	}

	descriptions := make([]client.CollectionDescription, len(collections))
	for i, collection := range collections {
		descriptions[i] = collection.Description()
	}

	return descriptions, nil
}

// patchSchema takes the given JSON patch string and applies it to the set of CollectionDescriptions
// present in the database.
//
// It will also update the GQL types used by the query system. It will error and not apply any of the
// requested, valid updates should the net result of the patch result in an invalid state.  The
// individual operations defined in the patch do not need to result in a valid state, only the net result
// of the full patch.
//
// The collections (including the schema version ID) will only be updated if any changes have actually
// been made, if the net result of the patch matches the current persisted description then no changes
// will be applied.
func (db *db) patchSchema(ctx context.Context, txn datastore.Txn, patchString string) error {
	patch, err := jsonpatch.DecodePatch([]byte(patchString))
	if err != nil {
		return err
	}
	// Here we swap out any string representations of enums for their integer values
	patch, err = substituteSchemaPatch(patch)
	if err != nil {
		return err
	}

	collectionsByName, err := db.getCollectionsByName(ctx, txn)
	if err != nil {
		return err
	}

	existingDescriptionJson, err := json.Marshal(collectionsByName)
	if err != nil {
		return err
	}

	newDescriptionJson, err := patch.Apply(existingDescriptionJson)
	if err != nil {
		return err
	}

	var newDescriptionsByName map[string]client.CollectionDescription
	decoder := json.NewDecoder(strings.NewReader(string(newDescriptionJson)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&newDescriptionsByName)
	if err != nil {
		return err
	}

	newDescriptions := []client.CollectionDescription{}
	for _, desc := range newDescriptionsByName {
		newDescriptions = append(newDescriptions, desc)
	}

	for _, desc := range newDescriptions {
		if _, err := db.updateCollection(ctx, txn, desc); err != nil {
			return err
		}
	}

	return db.parser.SetSchema(ctx, txn, newDescriptions)
}

func (db *db) getCollectionsByName(
	ctx context.Context,
	txn datastore.Txn,
) (map[string]client.CollectionDescription, error) {
	collections, err := db.getAllCollections(ctx, txn)
	if err != nil {
		return nil, err
	}

	collectionsByName := map[string]client.CollectionDescription{}
	for _, collection := range collections {
		collectionsByName[collection.Name()] = collection.Description()
	}

	return collectionsByName, nil
}

// substituteSchemaPatch handles any substitution of values that may be required before
// the patch can be applied.
//
// For example Field [FieldKind] string representations will be replaced by the raw integer
// value.
func substituteSchemaPatch(patch jsonpatch.Patch) (jsonpatch.Patch, error) {
	for _, patchOperation := range patch {
		path, err := patchOperation.Path()
		if err != nil {
			return nil, err
		}

		if value, hasValue := patchOperation["value"]; hasValue {
			if isField(path) {
				// We unmarshal the full field-value into a map to ensure that all user
				// specified properties are maintained.
				var field map[string]any
				err = json.Unmarshal(*value, &field)
				if err != nil {
					return nil, err
				}

				if kind, isString := field["Kind"].(string); isString {
					substitute, substituteFound := client.FieldKindStringToEnumMapping[kind]
					if substituteFound {
						field["Kind"] = substitute
						substituteField, err := json.Marshal(field)
						if err != nil {
							return nil, err
						}

						substituteValue := json.RawMessage(substituteField)
						patchOperation["value"] = &substituteValue
					} else {
						return nil, NewErrFieldKindNotFound(kind)
					}
				}
			} else if isFieldKind(path) {
				var kind any
				err = json.Unmarshal(*value, &kind)
				if err != nil {
					return nil, err
				}

				if kind, isString := kind.(string); isString {
					substitute, substituteFound := client.FieldKindStringToEnumMapping[kind]
					if substituteFound {
						substituteKind, err := json.Marshal(substitute)
						if err != nil {
							return nil, err
						}

						substituteValue := json.RawMessage(substituteKind)
						patchOperation["value"] = &substituteValue
					} else {
						return nil, NewErrFieldKindNotFound(kind)
					}
				}
			}
		}
	}

	return patch, nil
}

// isField returns true if the given path points to a FieldDescription.
func isField(path string) bool {
	path = strings.TrimPrefix(path, "/")
	elements := strings.Split(path, "/")
	//nolint:goconst
	return len(elements) == 4 && elements[len(elements)-2] == "Fields" && elements[len(elements)-3] == "Schema"
}

// isField returns true if the given path points to a FieldDescription.Kind property.
func isFieldKind(path string) bool {
	path = strings.TrimPrefix(path, "/")
	elements := strings.Split(path, "/")
	return len(elements) == 5 &&
		elements[len(elements)-1] == "Kind" &&
		elements[len(elements)-3] == "Fields" &&
		elements[len(elements)-4] == "Schema"
}
