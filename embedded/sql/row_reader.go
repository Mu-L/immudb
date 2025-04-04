/*
Copyright 2025 Codenotary Inc. All rights reserved.

SPDX-License-Identifier: BUSL-1.1
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://mariadb.com/bsl11/

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sql

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/codenotary/immudb/embedded/store"
)

type RowReader interface {
	Tx() *SQLTx
	TableAlias() string
	Parameters() map[string]interface{}
	Read(ctx context.Context) (*Row, error)
	Close() error
	Columns(ctx context.Context) ([]ColDescriptor, error)
	OrderBy() []ColDescriptor
	ScanSpecs() *ScanSpecs
	InferParameters(ctx context.Context, params map[string]SQLValueType) error
	colsBySelector(ctx context.Context) (map[string]ColDescriptor, error)
	onClose(func())
}

type ScanSpecs struct {
	Index             *Index
	rangesByColID     map[uint32]*typedValueRange
	IncludeHistory    bool
	IncludeTxMetadata bool
	DescOrder         bool
	groupBySortExps   []*OrdExp
	orderBySortExps   []*OrdExp
}

func (s *ScanSpecs) extraCols() int {
	n := 0
	if s.IncludeHistory {
		n++
	}

	if s.IncludeTxMetadata {
		n++
	}
	return n
}

type Row struct {
	ValuesByPosition []TypedValue
	ValuesBySelector map[string]TypedValue
}

// rows are selector-compatible if both rows have the same assigned value for all specified selectors
func (row *Row) compatible(aRow *Row, selectors []*ColSelector, table string) (bool, error) {
	for _, sel := range selectors {
		c := EncodeSelector(sel.resolve(table))

		val1, ok := row.ValuesBySelector[c]
		if !ok {
			return false, ErrInvalidColumn
		}

		val2, ok := aRow.ValuesBySelector[c]
		if !ok {
			return false, ErrInvalidColumn
		}

		cmp, err := val1.Compare(val2)
		if err != nil {
			return false, err
		}

		if cmp != 0 {
			return false, nil
		}
	}

	return true, nil
}

func (row *Row) digest(cols []ColDescriptor) (d [sha256.Size]byte, err error) {
	h := sha256.New()

	for i, v := range row.ValuesByPosition {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(i))
		h.Write(b[:])

		_, isNull := v.(*NullValue)
		if isNull {
			continue
		}

		encVal, err := EncodeValue(v, v.Type(), 0)
		if err != nil {
			return d, err
		}

		h.Write(encVal)
	}

	copy(d[:], h.Sum(nil))

	return
}

type rawRowReader struct {
	tx         *SQLTx
	table      *Table
	tableAlias string
	colsByPos  []ColDescriptor
	colsBySel  map[string]ColDescriptor
	scanSpecs  *ScanSpecs

	// defines a sub-range a transactions based on a combination of tx IDs and timestamps
	// the query is resolved only taking into consideration that range of transactioins
	period period

	// underlying store supports reading entries within a range of txs
	// the range is calculated based on the period stmt, which is included here to support
	// lazy evaluation when parameters are available
	txRange *txRange

	params map[string]interface{}

	reader          store.KeyReader
	onCloseCallback func()
}

type txRange struct {
	initialTxID uint64
	finalTxID   uint64
}

type ColDescriptor struct {
	AggFn  string
	Table  string
	Column string
	Type   SQLValueType
}

func (d *ColDescriptor) Selector() string {
	return EncodeSelector(d.AggFn, d.Table, d.Column)
}

type emptyKeyReader struct {
}

func (r emptyKeyReader) Read(ctx context.Context) (key []byte, val store.ValueRef, err error) {
	return nil, nil, store.ErrNoMoreEntries
}

func (r emptyKeyReader) ReadBetween(ctx context.Context, initialTxID uint64, finalTxID uint64) (key []byte, val store.ValueRef, err error) {
	return nil, nil, store.ErrNoMoreEntries
}

func (r emptyKeyReader) Reset() error {
	return nil
}

func (r emptyKeyReader) Close() error {
	return nil
}

func newRawRowReader(tx *SQLTx, params map[string]interface{}, table *Table, period period, tableAlias string, scanSpecs *ScanSpecs) (*rawRowReader, error) {
	if table == nil || scanSpecs == nil || scanSpecs.Index == nil {
		return nil, ErrIllegalArguments
	}

	rSpec, err := keyReaderSpecFrom(tx.engine.prefix, table, scanSpecs)
	if err != nil {
		return nil, err
	}

	var r store.KeyReader

	if table.name == "pg_type" {
		r = &emptyKeyReader{}
	} else {
		r, err = tx.newKeyReader(*rSpec)
		if err != nil {
			return nil, err
		}
	}

	if tableAlias == "" {
		tableAlias = table.name
	}

	nCols := len(table.cols) + scanSpecs.extraCols()

	colsByPos := make([]ColDescriptor, nCols)
	colsBySel := make(map[string]ColDescriptor, nCols)

	off := 0
	if scanSpecs.IncludeHistory {
		colDescriptor := ColDescriptor{
			Table:  tableAlias,
			Column: revCol,
			Type:   IntegerType,
		}

		colsByPos[off] = colDescriptor
		colsBySel[colDescriptor.Selector()] = colDescriptor
		off++
	}

	if scanSpecs.IncludeTxMetadata {
		colDescriptor := ColDescriptor{
			Table:  tableAlias,
			Column: txMetadataCol,
			Type:   JSONType,
		}

		colsByPos[off] = colDescriptor
		colsBySel[colDescriptor.Selector()] = colDescriptor
		off++
	}

	for i, c := range table.cols {
		colDescriptor := ColDescriptor{
			Table:  tableAlias,
			Column: c.colName,
			Type:   c.colType,
		}

		colsByPos[off+i] = colDescriptor
		colsBySel[colDescriptor.Selector()] = colDescriptor
	}

	return &rawRowReader{
		tx:         tx,
		table:      table,
		period:     period,
		tableAlias: tableAlias,
		colsByPos:  colsByPos,
		colsBySel:  colsBySel,
		scanSpecs:  scanSpecs,
		params:     params,
		reader:     r,
	}, nil
}

func keyReaderSpecFrom(sqlPrefix []byte, table *Table, scanSpecs *ScanSpecs) (spec *store.KeyReaderSpec, err error) {
	prefix := MapKey(sqlPrefix, MappedPrefix, EncodeID(table.id), EncodeID(scanSpecs.Index.id))

	var loKey []byte
	var loKeyReady bool

	var hiKey []byte
	var hiKeyReady bool

	loKey = make([]byte, len(prefix))
	copy(loKey, prefix)

	hiKey = make([]byte, len(prefix))
	copy(hiKey, prefix)

	// seekKey and endKey in the loop below are scan prefixes for beginning
	// and end of the index scanning range. On each index we try to make them more
	// concrete.
	for _, col := range scanSpecs.Index.cols {
		colRange, ok := scanSpecs.rangesByColID[col.id]
		if !ok {
			break
		}

		if !hiKeyReady {
			if colRange.hRange == nil {
				hiKeyReady = true
			} else {
				encVal, _, err := EncodeValueAsKey(colRange.hRange.val, col.colType, col.MaxLen())
				if err != nil {
					return nil, err
				}
				hiKey = append(hiKey, encVal...)
			}
		}

		if !loKeyReady {
			if colRange.lRange == nil {
				loKeyReady = true
			} else {
				encVal, _, err := EncodeValueAsKey(colRange.lRange.val, col.colType, col.MaxLen())
				if err != nil {
					return nil, err
				}
				loKey = append(loKey, encVal...)
			}
		}
	}

	// Ensure the hiKey is inclusive regarding all values with that prefix
	hiKey = append(hiKey, KeyValPrefixUpperBound)

	seekKey := loKey
	endKey := hiKey

	if scanSpecs.DescOrder {
		seekKey, endKey = endKey, seekKey
	}

	return &store.KeyReaderSpec{
		SeekKey:        seekKey,
		InclusiveSeek:  true,
		EndKey:         endKey,
		InclusiveEnd:   true,
		Prefix:         prefix,
		DescOrder:      scanSpecs.DescOrder,
		Filters:        []store.FilterFn{store.IgnoreExpired, store.IgnoreDeleted},
		IncludeHistory: scanSpecs.IncludeHistory,
	}, nil
}

func (r *rawRowReader) onClose(callback func()) {
	r.onCloseCallback = callback
}

func (r *rawRowReader) Tx() *SQLTx {
	return r.tx
}

func (r *rawRowReader) TableAlias() string {
	return r.tableAlias
}

func (r *rawRowReader) OrderBy() []ColDescriptor {
	cols := make([]ColDescriptor, len(r.scanSpecs.Index.cols))

	for i, col := range r.scanSpecs.Index.cols {
		cols[i] = ColDescriptor{
			Table:  r.tableAlias,
			Column: col.colName,
			Type:   col.colType,
		}
	}

	return cols
}

func (r *rawRowReader) ScanSpecs() *ScanSpecs {
	return r.scanSpecs
}

func (r *rawRowReader) Columns(ctx context.Context) ([]ColDescriptor, error) {
	ret := make([]ColDescriptor, len(r.colsByPos))
	copy(ret, r.colsByPos)
	return ret, nil
}

func (r *rawRowReader) colsBySelector(ctx context.Context) (map[string]ColDescriptor, error) {
	ret := make(map[string]ColDescriptor, len(r.colsBySel))
	for sel := range r.colsBySel {
		ret[sel] = r.colsBySel[sel]
	}
	return ret, nil
}

func (r *rawRowReader) InferParameters(ctx context.Context, params map[string]SQLValueType) error {
	cols, err := r.colsBySelector(ctx)
	if err != nil {
		return err
	}

	if r.period.start != nil {
		_, err = r.period.start.instant.exp.inferType(cols, params, r.TableAlias())
		if err != nil {
			return err
		}
	}

	if r.period.end != nil {
		_, err = r.period.end.instant.exp.inferType(cols, params, r.TableAlias())
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *rawRowReader) Parameters() map[string]interface{} {
	return r.params
}

func (r *rawRowReader) reduceTxRange() (err error) {
	if r.txRange != nil || (r.period.start == nil && r.period.end == nil) {
		return nil
	}

	txRange := &txRange{
		initialTxID: uint64(0),
		finalTxID:   uint64(math.MaxUint64),
	}

	if r.period.start != nil {
		txRange.initialTxID, err = r.period.start.instant.resolve(r.tx, r.params, true, r.period.start.inclusive)
		if err != nil {
			return err
		}
	}

	if r.period.end != nil {
		txRange.finalTxID, err = r.period.end.instant.resolve(r.tx, r.params, false, r.period.end.inclusive)
		if err != nil {
			return err
		}
	}

	r.txRange = txRange

	return nil
}

func (r *rawRowReader) Read(ctx context.Context) (*Row, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	//var mkey []byte
	var vref store.ValueRef

	// evaluation of txRange is postponed to allow parameters to be provided after rowReader initialization
	err := r.reduceTxRange()
	if errors.Is(err, store.ErrTxNotFound) {
		return nil, ErrNoMoreRows
	}
	if err != nil {
		return nil, err
	}

	if r.txRange == nil {
		_, vref, err = r.reader.Read(ctx) //mkey
	} else {
		_, vref, err = r.reader.ReadBetween(ctx, r.txRange.initialTxID, r.txRange.finalTxID) //mkey
	}
	if err != nil {
		return nil, err
	}

	v, err := vref.Resolve()
	if err != nil {
		return nil, err
	}

	valuesByPosition := make([]TypedValue, len(r.colsByPos))
	valuesBySelector := make(map[string]TypedValue, len(r.colsBySel))

	for i, col := range r.colsByPos {
		var val TypedValue

		switch col.Column {
		case revCol:
			val = &Integer{val: int64(vref.HC())}
		case txMetadataCol:
			val, err = r.parseTxMetadata(vref.TxMetadata())
			if err != nil {
				return nil, err
			}
		default:
			val = &NullValue{t: col.Type}
		}

		valuesByPosition[i] = val
		valuesBySelector[col.Selector()] = val
	}

	if len(v) < EncLenLen {
		return nil, ErrCorruptedData
	}

	extraCols := r.scanSpecs.extraCols()

	voff := 0

	cols := int(binary.BigEndian.Uint32(v[voff:]))
	voff += EncLenLen

	for i, pos := 0, 0; i < cols; i++ {
		if len(v) < EncIDLen {
			return nil, ErrCorruptedData
		}

		colID := binary.BigEndian.Uint32(v[voff:])
		voff += EncIDLen

		col, err := r.table.GetColumnByID(colID)
		if errors.Is(err, ErrColumnDoesNotExist) && colID <= r.table.maxColID {
			// Dropped column, skip it
			vlen, n, err := DecodeValueLength(v[voff:])
			if err != nil {
				return nil, err
			}
			voff += n + vlen

			continue
		}
		if err != nil {
			return nil, ErrCorruptedData
		}

		val, n, err := DecodeValue(v[voff:], col.colType)
		if err != nil {
			return nil, err
		}

		voff += n

		// make sure value is inserted in the correct position
		for pos < len(r.table.cols) && r.table.cols[pos].id < colID {
			pos++
		}

		if pos == len(r.table.cols) || r.table.cols[pos].id != colID {
			return nil, ErrCorruptedData
		}

		valuesByPosition[pos+extraCols] = val

		pos++

		valuesBySelector[EncodeSelector("", r.tableAlias, col.colName)] = val
	}

	if len(v)-voff > 0 {
		return nil, ErrCorruptedData
	}

	return &Row{ValuesByPosition: valuesByPosition, ValuesBySelector: valuesBySelector}, nil
}

func (r *rawRowReader) parseTxMetadata(txmd *store.TxMetadata) (TypedValue, error) {
	if txmd == nil {
		return &NullValue{t: JSONType}, nil
	}

	if extra := txmd.Extra(); extra != nil {
		if r.tx.engine.parseTxMetadata == nil {
			return nil, fmt.Errorf("unable to parse tx metadata")
		}

		md, err := r.tx.engine.parseTxMetadata(extra)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidTxMetadata, err)
		}
		return &JSON{val: md}, nil
	}
	return &NullValue{t: JSONType}, nil
}

func (r *rawRowReader) Close() error {
	if r.onCloseCallback != nil {
		defer r.onCloseCallback()
	}

	return r.reader.Close()
}

func ReadAllRows(ctx context.Context, reader RowReader) ([]*Row, error) {
	var rows []*Row
	err := ReadRowsBatch(ctx, reader, 100, func(rowBatch []*Row) error {
		if rows == nil {
			rows = make([]*Row, 0, len(rowBatch))
		}
		rows = append(rows, rowBatch...)
		return nil
	})
	return rows, err
}

func ReadRowsBatch(ctx context.Context, reader RowReader, batchSize int, onBatch func([]*Row) error) error {
	rows := make([]*Row, batchSize)

	hasMoreRows := true
	for hasMoreRows {
		n, err := readNRows(ctx, reader, batchSize, rows)

		if n > 0 {
			if err := onBatch(rows[:n]); err != nil {
				return err
			}
		}

		hasMoreRows = !errors.Is(err, ErrNoMoreRows)
		if err != nil && hasMoreRows {
			return err
		}
	}
	return nil
}

func readNRows(ctx context.Context, reader RowReader, n int, outRows []*Row) (int, error) {
	for i := 0; i < n; i++ {
		r, err := reader.Read(ctx)
		if err != nil {
			return i, err
		}
		outRows[i] = r
	}
	return n, nil
}
