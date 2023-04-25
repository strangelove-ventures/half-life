package db

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"github.com/dgraph-io/badger/v4"
	"github.com/staking4all/celestia-monitoring-bot/services"
	"github.com/staking4all/celestia-monitoring-bot/services/models"
)

type badgerDB struct {
	db *badger.DB
}

func NewDB() (services.PersistenceDB, error) {
	// Open a new Badger DB instance
	options := badger.DefaultOptions(".badger_db")
	db, err := badger.Open(options)
	if err != nil {
		log.Fatalf("Failed to open Badger DB: %v", err)
	}

	return &badgerDB{db: db}, nil
}

func (b *badgerDB) Close() error {
	return b.db.Close()
}

func getKey(userID int64, address string) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(userID))

	return append(b, []byte(address)...)
}

func getUserFromKey(key []byte) (int64, error) {
	if len(key) < 8 {
		return 0, fmt.Errorf("corrupt database: invalid key %s", hex.EncodeToString(key))
	}

	v := binary.LittleEndian.Uint64(key[:8])

	return int64(v), nil
}

func (b *badgerDB) Add(userID int64, v *models.Validator) error {
	key := getKey(userID, v.Address)
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// Set a key-value pair in the database
	err = b.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, data)
		return err
	})

	return err
}

func (b *badgerDB) Remove(userID int64, address string) error {
	key := getKey(userID, address)
	// Remove the key from the database
	err := b.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		return err
	})

	return err
}

func (b *badgerDB) List() (map[int64][]models.Validator, error) {
	result := make(map[int64][]models.Validator)
	// List all keys
	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			userID, err := getUserFromKey(key)
			if err != nil {
				return err
			}
			v := &models.Validator{}
			// decode
			err = json.Unmarshal(value, v)
			if err != nil {
				return err
			}

			if result[userID] == nil {
				result[userID] = make([]models.Validator, 0)
			}

			result[userID] = append(result[userID], *v)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
