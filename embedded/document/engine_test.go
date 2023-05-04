/*
Copyright 2023 Codenotary Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package document

import (
	"context"
	"testing"

	"github.com/codenotary/immudb/embedded/sql"
	"github.com/codenotary/immudb/embedded/store"
	"github.com/codenotary/immudb/pkg/api/protomodel"
	"github.com/stretchr/testify/require"
)

func makeEngine(t *testing.T) *Engine {
	st, err := store.Open(t.TempDir(), store.DefaultOptions())
	require.NoError(t, err)

	t.Cleanup(func() {
		err := st.Close()
		if !t.Failed() {
			// Do not pollute error output if test has already failed
			require.NoError(t, err)
		}
	})

	opts := sql.DefaultOptions()
	engine, err := NewEngine(st, opts)
	require.NoError(t, err)

	return engine
}

func TestListCollections(t *testing.T) {
	engine := makeEngine(t)

	collections := []string{"mycollection1", "mycollection2", "mycollection3"}

	for _, collectionName := range collections {
		err := engine.CreateCollection(
			context.Background(),
			collectionName,
			"",
			[]*protomodel.Field{
				{Name: "number", Type: protomodel.FieldType_INTEGER},
				{Name: "name", Type: protomodel.FieldType_STRING},
				{Name: "pin", Type: protomodel.FieldType_INTEGER},
				{Name: "country", Type: protomodel.FieldType_STRING},
			},
			[]*protomodel.Index{
				{Fields: []string{"number"}},
				{Fields: []string{"name"}},
				{Fields: []string{"pin"}},
				{Fields: []string{"country"}},
			},
		)
		require.NoError(t, err)
	}

	collectionList, err := engine.ListCollections(context.Background())
	require.NoError(t, err)
	require.Equal(t, len(collections), len(collectionList))
}

/*
func TestCreateCollection(t *testing.T) {
	engine := makeEngine(t)

	collectionName := "mycollection"
	err := engine.CreateCollection(
		context.Background(),
		collectionName,
		map[string]*IndexOption{
			"number":  {Type: sql.Float64Type},
			"name":    {Type: sql.VarcharType},
			"pin":     {Type: sql.IntegerType},
			"country": {Type: sql.VarcharType},
		},
	)
	require.NoError(t, err)

	// creating collection with the same name should throw error
	err = engine.CreateCollection(
		context.Background(),
		collectionName,
		nil,
	)
	require.ErrorIs(t, err, sql.ErrTableAlreadyExists)

	catalog, err := engine.sqlEngine.Catalog(context.Background(), nil)
	require.NoError(t, err)

	table, err := catalog.GetTableByName(collectionName)
	require.NoError(t, err)

	require.Equal(t, collectionName, table.Name())

	pcols := []string{"_id"}
	idxcols := []string{"pin", "country", "number", "name"}

	// verify primary keys
	for _, col := range pcols {
		c, err := table.GetColumnByName(col)
		require.NoError(t, err)
		require.Equal(t, c.Name(), col)
	}

	// verify index keys
	for _, col := range idxcols {
		c, err := table.GetColumnByName(col)
		require.NoError(t, err)
		require.Equal(t, c.Name(), col)
	}

	// get collection
	indexes, err := engine.GetCollection(context.Background(), collectionName)
	require.NoError(t, err)
	require.Equal(t, 5, len(indexes))

	primaryKeyCount := 0
	indexKeyCount := 0
	for _, idx := range indexes {
		// check if primary key
		if idx.IsPrimary() {
			primaryKeyCount += len(idx.Cols())
		} else {
			indexKeyCount += len(idx.Cols())
		}
	}
	require.Equal(t, 1, primaryKeyCount)
	require.Equal(t, 4, indexKeyCount)
}

func TestGetDocument(t *testing.T) {
	ctx := context.Background()
	engine := makeEngine(t)
	// create collection
	collectionName := "mycollection"
	err := engine.CreateCollection(context.Background(), collectionName, map[string]*IndexOption{
		"pincode": {Type: sql.IntegerType},
		"country": {Type: sql.VarcharType},
		"data":    {Type: sql.BLOBType},
	})
	require.NoError(t, err)

	// add document to collection
	_, _, err = engine.InsertDocument(context.Background(), collectionName, &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"pincode": {
				Kind: &structpb.Value_NumberValue{NumberValue: 2},
			},
			"country": {
				Kind: &structpb.Value_StringValue{StringValue: "wonderland"},
			},
			"data": {
				Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": {Kind: &structpb.Value_StringValue{StringValue: "value1"}},
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	expressions := []*Query{
		{
			Field:    "country",
			Operator: 0, // EQ
			Value: &structpb.Value{
				Kind: &structpb.Value_StringValue{StringValue: "wonderland"},
			},
		},
		{
			Field:    "pincode",
			Operator: 0, // EQ
			Value: &structpb.Value{
				Kind: &structpb.Value_NumberValue{NumberValue: 2},
			},
		},
	}

	reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
	require.NoError(t, err)
	defer reader.Close()
	docs, err := reader.Read(ctx, 1)
	require.NoError(t, err)

	require.Equal(t, 1, len(docs))
}

func TestDocumentAudit(t *testing.T) {
	engine := makeEngine(t)

	// create collection
	collectionName := "mycollection"
	err := engine.CreateCollection(context.Background(), collectionName, map[string]*IndexOption{
		"pincode": {Type: sql.IntegerType},
		"country": {Type: sql.VarcharType},
	})
	require.NoError(t, err)

	// add document to collection
	docID, _, err := engine.InsertDocument(context.Background(), collectionName, &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"pincode": {
				Kind: &structpb.Value_NumberValue{NumberValue: 2},
			},
			"country": {
				Kind: &structpb.Value_StringValue{StringValue: "wonderland"},
			},
			"data": {
				Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": {Kind: &structpb.Value_StringValue{StringValue: "value1"}},
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	// Prepare a query to find the document
	queries := []*Query{newQuery("country", sql.EQ, &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: "wonderland"},
	})}

	_, revision, err := engine.UpdateDocument(context.Background(), collectionName, queries, &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"_id": {
				Kind: &structpb.Value_StringValue{StringValue: docID.EncodeToHexString()},
			},
			"pincode": {
				Kind: &structpb.Value_NumberValue{NumberValue: 2},
			},
			"country": {
				Kind: &structpb.Value_StringValue{StringValue: "wonderland"},
			},
			"data": {
				Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": {Kind: &structpb.Value_StringValue{StringValue: "value2"}},
					},
				}},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), revision)

	// get document audit
	res, err := engine.DocumentAudit(context.Background(), collectionName, docID, 1, 10)
	require.NoError(t, err)
	require.Equal(t, 2, len(res))

	for i, docAudit := range res {
		require.Contains(t, docAudit.Document.Fields, DocumentIDField)
		require.Contains(t, docAudit.Document.Fields, "pincode")
		require.Contains(t, docAudit.Document.Fields, "country")
		require.Equal(t, uint64(i+1), docAudit.Revision)
	}
}

func TestQueryDocuments(t *testing.T) {
	ctx := context.Background()
	engine := makeEngine(t)

	// create collection
	collectionName := "mycollection"
	err := engine.CreateCollection(context.Background(), collectionName, map[string]*IndexOption{
		"pincode": {Type: sql.IntegerType},
		"country": {Type: sql.VarcharType},
		"idx":     {Type: sql.IntegerType},
	})
	require.NoError(t, err)

	// add documents to collection
	for i := 1.0; i <= 10; i++ {
		_, _, err = engine.InsertDocument(context.Background(), collectionName, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"pincode": {
					Kind: &structpb.Value_NumberValue{NumberValue: i},
				},
				"country": {
					Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("country-%d", int(i))},
				},
				"idx": {
					Kind: &structpb.Value_NumberValue{NumberValue: i},
				},
			},
		})
		require.NoError(t, err)
	}

	t.Run("test query with != operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.NE,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 5},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 9, len(docs))
	})

	t.Run("test query with < operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.LT,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 11},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 10, len(docs))
	})

	t.Run("test query with <= operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.LE,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 9},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 9, len(docs))
	})

	t.Run("test query with > operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.GT,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 5},
				},
			},
		}
		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 5, len(docs))
	})

	t.Run("test query with >= operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.GE,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 10},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 1, len(docs))
	})

	t.Run("test group query with != operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.NE,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 5},
				},
			},
			{
				Field:    "country",
				Operator: sql.NE,
				Value: &structpb.Value{
					Kind: &structpb.Value_StringValue{StringValue: "country-6"},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 8, len(docs))
	})

	t.Run("test group query with < operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.LT,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 11},
				},
			},
			{
				Field:    "idx",
				Operator: sql.LT,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 5},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 4, len(docs))
	})

	t.Run("test group query with > operator", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "idx",
				Operator: sql.GT,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 7},
				},
			},
			{
				Field:    "pincode",
				Operator: sql.GT,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 5},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()
		docs, err := reader.Read(ctx, 20)
		require.ErrorIs(t, err, ErrNoMoreDocuments)
		require.Equal(t, 3, len(docs))
	})

}

func TestDocumentUpdate(t *testing.T) {
	// Create a new engine instance
	ctx := context.Background()
	e := makeEngine(t)

	// create collection
	// Create a test collection with a single document
	collectionName := "test_collection"
	err := e.CreateCollection(ctx, collectionName, map[string]*IndexOption{
		"name": {Type: sql.VarcharType},
		"age":  {Type: sql.Float64Type},
	})
	require.NoError(t, err)

	doc := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"name": {Kind: &structpb.Value_StringValue{StringValue: "Alice"}},
			"age":  {Kind: &structpb.Value_NumberValue{NumberValue: 30}},
		},
	}

	docID, _, _, err := e.upsertDocument(ctx, collectionName, doc, &upsertOptions{isInsert: true})
	if err != nil {
		t.Fatalf("Failed to insert test document: %v", err)
	}

	// Prepare a query to find the document by name
	queries := []*Query{newQuery("name", sql.EQ, &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: "Alice"},
	})}

	t.Run("update document should pass without docID", func(t *testing.T) {
		// Prepare a document to update the age field
		toUpdateDoc := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"name": {Kind: &structpb.Value_StringValue{StringValue: "Alice"}},
				"age":  {Kind: &structpb.Value_NumberValue{NumberValue: 31}},
			},
		}

		// Call the UpdateDocument method
		txID, rev, err := e.UpdateDocument(ctx, collectionName, queries, toUpdateDoc)
		require.NoError(t, err)
		// Check that the method returned the expected values
		require.NotEqual(t, txID, 0)
		require.NotEqual(t, rev, 0)

		// Verify that the document was updated
		reader, err := e.GetDocuments(ctx, collectionName, queries, 0)
		require.NoError(t, err)
		defer reader.Close()

		updatedDocs, err := reader.Read(ctx, 1)
		require.NoError(t, err)

		updatedDoc := updatedDocs[0]
		if updatedDoc.Fields["age"].GetNumberValue() != 31 {
			t.Errorf("Expected age to be updated to 31, got %v", updatedDoc.Fields["age"].GetNumberValue())
		}

		require.Equal(t, docID.EncodeToHexString(), updatedDoc.Fields[DocumentIDField].GetStringValue())

	})

	t.Run("update document should fail when no document is found", func(t *testing.T) {
		toUpdateDoc := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"name": {Kind: &structpb.Value_StringValue{StringValue: "Alice"}},
				"age":  {Kind: &structpb.Value_NumberValue{NumberValue: 31}},
			},
		}

		// Test error case when no documents are found
		queries = []*Query{newQuery("name", sql.EQ, &structpb.Value{
			Kind: &structpb.Value_StringValue{StringValue: "Bob"},
		})}
		_, _, err = e.UpdateDocument(ctx, collectionName, queries, toUpdateDoc)
		if !errors.Is(err, ErrDocumentNotFound) {
			t.Errorf("Expected ErrDocumentNotFound, got %v", err)
		}
	})

	t.Run("update document should fail with a different docID", func(t *testing.T) {
		toUpdateDoc := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"_id":  {Kind: &structpb.Value_StringValue{StringValue: "123"}},
				"name": {Kind: &structpb.Value_StringValue{StringValue: "Alice"}},
				"age":  {Kind: &structpb.Value_NumberValue{NumberValue: 34}},
			},
		}
		queries = []*Query{newQuery("name", sql.EQ, &structpb.Value{
			Kind: &structpb.Value_StringValue{StringValue: "Alice"},
		})}

		// Call the UpdateDocument method
		_, _, err := e.UpdateDocument(ctx, collectionName, queries, toUpdateDoc)
		require.ErrorIs(t, err, ErrDocumentIDMismatch)
	})
}

func TestFloatSupport(t *testing.T) {
	ctx := context.Background()
	engine := makeEngine(t)

	collectionName := "mycollection"
	err := engine.CreateCollection(
		ctx,
		collectionName,
		map[string]*IndexOption{
			"number": {Type: sql.Float64Type},
		},
	)
	require.NoError(t, err)

	catalog, err := engine.sqlEngine.Catalog(ctx, nil)
	require.NoError(t, err)

	table, err := catalog.GetTableByName(collectionName)
	require.NoError(t, err)
	require.Equal(t, collectionName, table.Name())

	col, err := table.GetColumnByName("number")
	require.NoError(t, err)
	require.Equal(t, sql.Float64Type, col.Type())

	// add document to collection
	_, _, err = engine.InsertDocument(context.Background(), collectionName, &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"number": {
				Kind: &structpb.Value_NumberValue{NumberValue: 3.1},
			},
		},
	})
	require.NoError(t, err)

	// query document
	expressions := []*Query{
		{
			Field:    "number",
			Operator: sql.EQ,
			Value: &structpb.Value{
				Kind: &structpb.Value_NumberValue{NumberValue: 3.1},
			},
		},
	}

	// check if document is updated
	reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
	require.NoError(t, err)
	defer reader.Close()
	docs, err := reader.Read(ctx, 1)
	require.NoError(t, err)

	require.Equal(t, 1, len(docs))

	// retrieve document
	doc := docs[0]
	require.Equal(t, 3.1, doc.Fields["number"].GetNumberValue())
}

func TestDeleteCollection(t *testing.T) {
	engine := makeEngine(t)

	// create collection
	collectionName := "mycollection"
	err := engine.CreateCollection(context.Background(), collectionName, map[string]*IndexOption{
		"idx": {Type: sql.IntegerType},
	})
	require.NoError(t, err)

	// add documents to collection
	for i := 1.0; i <= 10; i++ {
		_, _, err = engine.InsertDocument(context.Background(), collectionName, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"idx": {
					Kind: &structpb.Value_NumberValue{NumberValue: i},
				},
			},
		})
		require.NoError(t, err)
	}

	t.Run("delete collection and check if it is empty", func(t *testing.T) {
		err = engine.DeleteCollection(context.Background(), collectionName)
		require.NoError(t, err)

		_, err := engine.sqlEngine.Query(context.Background(), nil, "SELECT COUNT(*) FROM mycollection", nil)
		require.ErrorIs(t, err, sql.ErrTableDoesNotExist)

		collectionList, err := engine.ListCollections(context.Background())
		require.NoError(t, err)
		require.Equal(t, 0, len(collectionList))
	})
}

func TestUpdateCollection(t *testing.T) {
	engine := makeEngine(t)

	collectionName := "mycollection"

	t.Run("create collection and add index", func(t *testing.T) {
		err := engine.CreateCollection(
			context.Background(),
			collectionName,
			map[string]*IndexOption{
				"number":  {Type: sql.Float64Type},
				"name":    {Type: sql.VarcharType},
				"pin":     {Type: sql.IntegerType},
				"country": {Type: sql.VarcharType},
			},
		)
		require.NoError(t, err)
	})

	t.Run("update collection by deleting indexes", func(t *testing.T) {
		// update collection
		err := engine.UpdateCollection(
			context.Background(),
			collectionName,
			nil,
			[]string{"number", "name"},
		)
		require.NoError(t, err)

		// get collection
		indexes, err := engine.GetCollection(context.Background(), collectionName)
		require.NoError(t, err)
		require.Equal(t, 3, len(indexes))

		primaryKeyCount := 0
		indexKeyCount := 0
		for _, idx := range indexes {
			// check if primary key
			if idx.IsPrimary() {
				primaryKeyCount += len(idx.Cols())
			} else {
				indexKeyCount += len(idx.Cols())
			}
		}
		require.Equal(t, 1, primaryKeyCount)
		require.Equal(t, 2, indexKeyCount)

	})

	t.Run("update collection by adding indexes", func(t *testing.T) {
		// update collection
		err := engine.UpdateCollection(
			context.Background(),
			collectionName,
			map[string]*IndexOption{
				"data1": {Type: sql.VarcharType},
				"data2": {Type: sql.VarcharType},
				"data3": {Type: sql.VarcharType},
			},
			nil,
		)
		require.NoError(t, err)

		// get collection
		indexes, err := engine.GetCollection(context.Background(), collectionName)
		require.NoError(t, err)
		require.Equal(t, 6, len(indexes))

		primaryKeyCount := 0
		indexKeyCount := 0
		for _, idx := range indexes {
			// check if primary key
			if idx.IsPrimary() {
				primaryKeyCount += len(idx.Cols())
			} else {
				indexKeyCount += len(idx.Cols())
			}
		}
		require.Equal(t, 1, primaryKeyCount)
		require.Equal(t, 5, indexKeyCount)
	})
}

func TestCollectionUpdateWithDeletedIndex(t *testing.T) {
	engine := makeEngine(t)

	collectionName := "mycollection"

	t.Run("create collection and add index", func(t *testing.T) {
		err := engine.CreateCollection(
			context.Background(),
			collectionName,
			map[string]*IndexOption{
				"number": {Type: sql.Float64Type},
			},
		)
		require.NoError(t, err)
	})

	t.Run("update collection by deleting indexes", func(t *testing.T) {
		// update collection
		err := engine.UpdateCollection(
			context.Background(),
			collectionName,
			nil,
			[]string{"number"},
		)
		require.NoError(t, err)

		// get collection
		indexes, err := engine.GetCollection(context.Background(), collectionName)
		require.NoError(t, err)
		require.Equal(t, 1, len(indexes))

		primaryKeyCount := 0
		indexKeyCount := 0
		for _, idx := range indexes {
			// check if primary key
			if idx.IsPrimary() {
				primaryKeyCount += len(idx.Cols())
			} else {
				indexKeyCount += len(idx.Cols())
			}
		}
		require.Equal(t, 1, primaryKeyCount)
		require.Equal(t, 0, indexKeyCount)

	})

	t.Run("update collection by adding the same index should pass", func(t *testing.T) {
		// update collection
		err := engine.UpdateCollection(
			context.Background(),
			collectionName,
			map[string]*IndexOption{
				"number": {Type: sql.Float64Type},
			},
			nil,
		)
		require.NoError(t, err)

		// get collection
		indexes, err := engine.GetCollection(context.Background(), collectionName)
		require.NoError(t, err)
		require.Equal(t, 2, len(indexes))

		primaryKeyCount := 0
		indexKeyCount := 0
		for _, idx := range indexes {
			// check if primary key
			if idx.IsPrimary() {
				primaryKeyCount += len(idx.Cols())
			} else {
				indexKeyCount += len(idx.Cols())
			}
		}
		require.Equal(t, 1, primaryKeyCount)
		require.Equal(t, 1, indexKeyCount)
	})
}

func TestBulkInsert(t *testing.T) {
	ctx := context.Background()
	engine := makeEngine(t)

	// create collection
	collectionName := "mycollection"
	err := engine.CreateCollection(ctx, collectionName, map[string]*IndexOption{
		"country": {Type: sql.VarcharType},
		"price":   {Type: sql.Float64Type},
	})
	require.NoError(t, err)

	// add documents to collection
	docs := make([]*structpb.Struct, 0)
	for i := 1.0; i <= 10; i++ {
		doc := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"country": {
					Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("country-%d", int(i))},
				},
				"price": {
					Kind: &structpb.Value_NumberValue{NumberValue: i},
				},
			},
		}
		docs = append(docs, doc)
	}

	docIDs, txID, err := engine.BulkInsertDocuments(ctx, collectionName, docs)
	require.NoError(t, err)

	require.Equal(t, 10, len(docIDs))
	require.Equal(t, uint64(2), txID)

	expressions := []*Query{
		{
			Field:    "price",
			Operator: sql.GE, // EQ
			Value: &structpb.Value{
				Kind: &structpb.Value_NumberValue{NumberValue: 0},
			},
		},
	}

	reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
	require.NoError(t, err)
	defer reader.Close()

	docs, _ = reader.Read(ctx, 10)
	require.Equal(t, 10, len(docs))

	for i, doc := range docs {
		require.Equal(t, float64(i+1), doc.Fields["price"].GetNumberValue())
	}
}

func newQuery(field string, op int, value *structpb.Value) *Query {
	return &Query{
		Field:    field,
		Operator: op,
		Value:    value,
	}
}

func TestPaginationOnReader(t *testing.T) {
	ctx := context.Background()
	engine := makeEngine(t)

	// create collection
	collectionName := "mycollection"
	err := engine.CreateCollection(ctx, collectionName, map[string]*IndexOption{
		"idx":     {Type: sql.IntegerType},
		"country": {Type: sql.VarcharType},
		"pincode": {Type: sql.IntegerType},
	})
	require.NoError(t, err)

	// add documents to collection
	for i := 1.0; i <= 20; i++ {
		_, _, err = engine.InsertDocument(ctx, collectionName, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"pincode": {
					Kind: &structpb.Value_NumberValue{NumberValue: i},
				},
				"country": {
					Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("country-%d", int(i))},
				},
				"idx": {
					Kind: &structpb.Value_NumberValue{NumberValue: i},
				},
			},
		})
		require.NoError(t, err)
	}

	t.Run("test reader for multiple reads", func(t *testing.T) {
		expressions := []*Query{
			{
				Field:    "pincode",
				Operator: sql.GE,
				Value: &structpb.Value{
					Kind: &structpb.Value_NumberValue{NumberValue: 0},
				},
			},
		}

		reader, err := engine.GetDocuments(ctx, collectionName, expressions, 0)
		require.NoError(t, err)
		defer reader.Close()

		results := make([]*structpb.Struct, 0)
		// use the reader to read paginated documents 5 at a time
		for i := 0; i < 4; i++ {
			docs, _ := reader.Read(ctx, 5)
			require.Equal(t, 5, len(docs))
			results = append(results, docs...)
		}

		for i := 1.0; i <= 20; i++ {
			doc := results[int(i-1)]
			require.Equal(t, i, doc.Fields["idx"].GetNumberValue())
		}
	})
}
*/