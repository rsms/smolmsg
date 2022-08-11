// SPDX-License-Identifier: Apache-2.0
package main

import (
  "database/sql"
  "sync"

  _ "modernc.org/sqlite"
)

// type DBScannable interface {
//   Scan(dest ...any) error
// }

type DB struct {
  *sql.DB
  mu sync.RWMutex
}

func (db *DB) Open() error {
  conn, err := sql.Open("sqlite", DBFILE)
  if err != nil {
    return err
  }
  db.DB = conn
  return db.init()
}

func (db *DB) init() error {
  db.mu.Lock()
  defer db.mu.Unlock()
  _, err := db.Exec(`
  CREATE TABLE IF NOT EXISTS messages (
    id       blob not null primary key,
    subject  text,
    fromaddr text,
    toaddr   text,
    body     text,
    isread   int
  ) WITHOUT ROWID;
  CREATE TABLE IF NOT EXISTS authors (
    address  text not null primary key,
    name     text not null
  ) WITHOUT ROWID;
  `)
  return err
}

func (db *DB) Close() error {
  db.mu.Lock()
  defer db.mu.Unlock()
  if db.DB == nil {
    return nil
  }
  err := db.DB.Close()
  db.DB = nil
  return err
}

func (db *DB) LoadLatestMessage(msg *Message) error {
  db.mu.RLock()
  defer db.mu.RUnlock()
  row := db.QueryRow(`
    SELECT id, subject, fromaddr, authors.name as fromname
    FROM messages
    LEFT JOIN authors ON authors.address = messages.fromaddr
    ORDER BY id DESC
    LIMIT 1;
  `)
  return db.InitMessageRow4(msg, row)
}

// id, subject, fromaddr, fromname
func (db *DB) InitMessageRow4(msg *Message, row *sql.Row) error {
  id := msg.id[:]
  if err := row.Scan(&id, &msg.subject, &msg.from.address, &msg.from.name); err != nil {
    return err
  }
  if len(id) > 24 {
    return errorf("invalid id %q", id)
  }
  copy(msg.id[:24], id)
  msg.SetTimeFromId()
  return nil
}

// id, subject, fromaddr, fromname
func (db *DB) InitMessageRows4(msg *Message, rows *sql.Rows) error {
  // Note: "id := m.id[:0]; scan(&id)" doesn't work for some reason;
  // we get back a heap-allocated slice. I.e. the database driver does not
  // populate the m.id array. To avoid lots of little allocations we use
  // sql.RawBytes which gives back a borrowed reference to db-owned data.
  var id sql.RawBytes
  if err := rows.Scan(&id, &msg.subject, &msg.from.address, &msg.from.name); err != nil {
    return err
  }
  if len(id) > 24 {
    return errorf("invalid id %q", id)
  }
  copy(msg.id[:24], id)
  msg.SetTimeFromId()
  return nil
}

func (db *DB) PutMessage(msg *Message) error {
  db.mu.Lock()
  defer db.mu.Unlock()

  tx, err := db.Begin()
  if err != nil {
    return err
  }

  _, err = db.Exec(`
    INSERT OR IGNORE into messages
    (id, subject, fromaddr, toaddr, body) VALUES(?, ?, ?, ?, ?)
  `, msg.id[:], msg.subject, msg.from.address, msg.to.address, msg.body)
  if err != nil {
    return err
  }

  if msg.from.name != "" {
    _, err = db.Exec(`
      INSERT OR REPLACE into authors (address, name) VALUES(?, ?)
    `, msg.from.address, msg.from.name)
  } else {
    _, err = db.Exec(`
      INSERT OR IGNORE into authors (address, name) VALUES(?, '')
    `, msg.from.address)
  }
  if err != nil {
    _ = tx.Rollback()
    return err
  }

  return tx.Commit()
}
