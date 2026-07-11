package service

import (
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// servicesBucket 是 bbolt 中用于存储服务运行态的桶名。
const servicesBucket = "services"

// Registry 提供 bbolt 持久化的服务运行态访问接口。
// 所有方法线程安全（通过 bbolt 事务串行化）。
type Registry struct {
	db *bolt.DB
}

// Open 打开（或创建）一个基于 bolt.DB 的 Registry。
// 桶不存在时自动创建。
func Open(db *bolt.DB) (*Registry, error) {
	if db == nil {
		return nil, fmt.Errorf("db 不能为空")
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(servicesBucket))
		return err
	}); err != nil {
		return nil, fmt.Errorf("初始化 services bucket 失败: %w", err)
	}
	return &Registry{db: db}, nil
}

// Close 不持有 bolt.DB 的关闭权，由调用方负责（避免与 backend 双重关闭）。
func (r *Registry) Close() error { return nil }

// Get 读取单条运行态记录。键不存在时返回 (nil, nil)。
func (r *Registry) Get(name string) (*Record, error) {
	var rec *Record
	err := r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(servicesBucket))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(name))
		if raw == nil {
			return nil
		}
		var rr Record
		if err := json.Unmarshal(raw, &rr); err != nil {
			return fmt.Errorf("解码记录 %s 失败: %w", name, err)
		}
		rec = &rr
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// Upsert 写入或更新一条记录。Record.Name 用作 bbolt key。
func (r *Registry) Upsert(rec *Record) error {
	if rec == nil {
		return fmt.Errorf("记录不能为空")
	}
	if rec.Name == "" {
		return fmt.Errorf("记录 Name 不能为空")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("序列化记录失败: %w", err)
	}
	return r.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(servicesBucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(rec.Name), data)
	})
}

// Delete 删除一条记录。键不存在时静默忽略。
func (r *Registry) Delete(name string) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(servicesBucket))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(name))
	})
}

// List 返回所有运行态记录的有序列表（按 Name 排序）。
func (r *Registry) List() ([]*Record, error) {
	var out []*Record
	err := r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(servicesBucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rr Record
			if err := json.Unmarshal(v, &rr); err != nil {
				return fmt.Errorf("解码记录失败: %w", err)
			}
			out = append(out, &rr)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
