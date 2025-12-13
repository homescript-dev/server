package storage

import (
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

var stateBucket = []byte("state")

// Storage manages persistent state using bbolt
type Storage struct {
	db *bbolt.DB
}

// New creates a new Storage instance
func New(path string) (*Storage, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(stateBucket)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}

	return &Storage{db: db}, nil
}

// Get retrieves a value from storage
func (s *Storage) Get(key string) (interface{}, error) {
	var value interface{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(stateBucket)
		data := b.Get([]byte(key))
		if data == nil {
			return fmt.Errorf("key not found: %s", key)
		}
		return json.Unmarshal(data, &value)
	})
	return value, err
}

// Set stores a value in storage
func (s *Storage) Set(key string, value interface{}) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(stateBucket)
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), data)
	})
}

// Delete removes a value from storage
func (s *Storage) Delete(key string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(stateBucket)
		return b.Delete([]byte(key))
	})
}

// List returns all keys with a given prefix
func (s *Storage) List(prefix string) ([]string, error) {
	var keys []string
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(stateBucket)
		c := b.Cursor()

		prefixBytes := []byte(prefix)
		for k, _ := c.Seek(prefixBytes); k != nil && len(k) >= len(prefixBytes); k, _ = c.Next() {
			// Check if key starts with prefix
			match := true
			for i := 0; i < len(prefixBytes); i++ {
				if k[i] != prefixBytes[i] {
					match = false
					break
				}
			}
			if match {
				keys = append(keys, string(k))
			} else {
				break
			}
		}
		return nil
	})
	return keys, err
}

// Close closes the database
func (s *Storage) Close() error {
	return s.db.Close()
}
