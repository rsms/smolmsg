// SPDX-License-Identifier: Apache-2.0
package main

import (
  "io"
  "io/fs"
  "os"
  "path/filepath"
  "strings"
  "sync"
  "sync/atomic"
)

var syncOldMessagesArray []*Message // TODO remove

type MessageSyncer struct {
  shutdown   uint32
  initscanwg sync.WaitGroup
}

func (ms *MessageSyncer) Start() {
  dlog("[sync] start")
  RegisterExitHandler(ms.Shutdown)
  ms.initscanwg.Add(1)
  go ms.main()
}

func (ms *MessageSyncer) WaitReady() {
  ms.initscanwg.Wait()
}

func (ms *MessageSyncer) main() {
  // initial file system scan of MSGDIR
  scanner := MessageFileScanner{}
  scanner.scanInbox()
  ms.initscanwg.Done()
}

func (ms *MessageSyncer) Shutdown() error {
  if !atomic.CompareAndSwapUint32(&ms.shutdown, 0, 1) {
    return nil // race lost or already shut down
  }
  // TODO
  return nil
}

type MessageFileScanner struct {
  wg  sync.WaitGroup
  err error
}

func (s *MessageFileScanner) scanInbox() {
  s.scanDir(INBOXDIR)
  s.wg.Wait() // wait for all operations to finish
  if s.err != nil {
    errlog("error in scanInbox: %v", s.err)
  }
}

func (s *MessageFileScanner) scanDir(dirpath string) {
  f, err := os.Open(dirpath)
  if err != nil {
    s.err = err
    return
  }
  defer f.Close()
  for {
    entries, err := f.ReadDir(64)
    if err != nil {
      if err != io.EOF {
        s.err = err
      }
      break
    }
    s.scanDirEntries(dirpath, entries)
  }
}

func (s *MessageFileScanner) scanDirEntries(dirpath string, entries []fs.DirEntry) {
  for _, ent := range entries {
    name := ent.Name()
    if name[0] == '.' { // skip dot files
      continue
    }
    path := filepath.Join(dirpath, name)
    if ent.IsDir() {
      s.scanDir(path)
    } else if strings.HasSuffix(name, ".msg") {
      s.wg.Add(1)
      go s.loadMessage(path)
    }
  }
}

func (s *MessageFileScanner) loadMessage(file string) {
  defer s.wg.Done()
  msg := &Message{}
  if err := msg.ParseFile(file); err != nil {
    logger.Printf("failed to read message file %q: %v", file, err)
    return
  }
  if err := db.PutMessage(msg); err != nil {
    errlog("failed to put message %s into database: %v", msg, err)
  }
}
