// SPDX-License-Identifier: Apache-2.0
package main

import (
  "io/fs"
  "os"
  "path/filepath"
  "sort"
)

func walkDirRev(dirpath string, callback fs.WalkDirFunc) error {
  return walkDirRev1(dirpath, nil, callback)
}

func walkDirRev1(dirpath string, d fs.DirEntry, callback fs.WalkDirFunc) error {
  if d != nil {
    if err := callback(dirpath, d, nil); err != nil || !d.IsDir() {
      if err == filepath.SkipDir && d.IsDir() {
        // Successfully skipped directory.
        err = nil
      }
      return err
    }
  }
  entries, err := readDirRev(dirpath)
  if err != nil {
    if d == nil {
      return err
    }
    err = callback(dirpath, d, err)
    if err != nil {
      if err == filepath.SkipDir && d.IsDir() {
        err = nil
      }
      return err
    }
  }
  for _, dirent := range entries {
    fullpath := filepath.Join(dirpath, dirent.Name())
    if err := walkDirRev1(fullpath, dirent, callback); err != nil {
      if err == filepath.SkipDir {
        break
      }
      return err
    }
  }
  return nil
}

func readDirRev(dirname string) ([]fs.DirEntry, error) {
  f, err := os.Open(dirname)
  if err != nil {
    return nil, err
  }
  dirs, err := f.ReadDir(-1)
  f.Close()
  if err != nil {
    return nil, err
  }
  sort.Slice(dirs, func(i, j int) bool { return dirs[j].Name() < dirs[i].Name() })
  return dirs, nil
}
