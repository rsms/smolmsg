// SPDX-License-Identifier: Apache-2.0
package main

import (
  "flag"
  "fmt"
  "log"
  "math"
  "os"
  "strings"
  "text/tabwriter"
  "time"
)

func cmd_list(args ...string) {
  const usagefmt = `
Usage: %s list [options]
List messages in inbox
Options:
  `
  fl := flag.NewFlagSet("list", flag.ExitOnError)
  fl.Usage = func() {
    fmt.Fprintf(os.Stderr, strings.TrimSpace(usagefmt)+"\n", progname)
    fl.PrintDefaults()
  }
  opt_nowait := fl.Bool("nowait", false, "Don't wait for inbox scan")
  fl.Parse(args)

  if !*opt_nowait {
    msgsync.WaitReady()
  }
  printMessageList(0, 20)
}

func printMessageList(offset, limit int) int {
  db.mu.RLock()
  defer db.mu.RUnlock()
  rows, err := db.Query(`
    SELECT id, subject, fromaddr, authors.name as fromname
    FROM messages
    LEFT JOIN authors ON authors.address = messages.fromaddr
    ORDER BY id DESC
    LIMIT ? OFFSET ?;
  `, limit, offset)
  if err != nil {
    log.Fatal(err)
  }
  defer rows.Close()

  coldim := "\x1B[2m"
  colrow := "\x1B[1m"
  colreset := "\x1B[0m"
  now := time.Now()
  padding := 2
  w := tabwriter.NewWriter(os.Stdout, 0, 0, padding, ' ', 0)
  var prevday, prevmonth, prevyear int
  numwidth := int(math.Log10(float64(offset + limit)))
  i := offset + limit
  fmt.Fprintf(w, "%s  # From\tSubject\tTime%s\n", coldim, colreset)

  for rows.Next() {
    var msg Message
    must(db.InitMessageRows4(&msg, rows))

    from := limitStrLen(msg.from.ShortString(), 20)
    subject := limitStrLen(msg.subject, 35)
    t := msg.time.Local()

    year := t.Year()
    month := (year << 14) | int(t.Month())
    day := (month << 12) | t.Day()

    if prevyear != 0 {
      if year != prevyear {
        fmt.Fprintf(w, "  %s%d\t\t%s\n", coldim, year, colreset)
      } else if month != prevmonth {
        fmt.Fprintf(w, "  %s%s\t\t%s\n", coldim, t.Month(), colreset)
      } else if day != prevday {
        fmt.Fprintf(w, "  %s%s\t\t%s\n", coldim, t.Weekday(), colreset)
      }
    }

    when := formatTime(now, t)
    marker := "●" // TODO unread or not
    fmt.Fprintf(w, "%s%s %*d %s\t%s\t%s%s\n",
      colrow, marker, numwidth, i, from, subject, when, colreset)

    prevyear = year
    prevmonth = month
    prevday = day

    i--
  }

  // if end > -1 {
  //   fmt.Fprintf(w, "  %s(%d more)\t\t%s\n", coldim, end+1, colreset)
  // }

  w.Flush()
  return (offset + limit) - i
}

func limitStrLen(s string, maxlen int) string {
  if len(s) > maxlen {
    return s[:maxlen-1] + "…"
  }
  return s
}

func formatTime(now time.Time, t time.Time) string {
  if now.Year() != t.Year() {
    return t.Format("2006, Jan 2, 15:04")
  }
  if now.Month() != t.Month() {
    return t.Format("Jan 2, 15:04")
  }
  if now.Day() != t.Day() {
    return t.Format("Jan 2, 15:04")
  }
  return t.Format("15:04:05")
}
