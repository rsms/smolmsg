# smolmsg

Simple and robust messaging

- Very simple protocol
- Plain text body with attachments. No MIME types, etc.
- Address includes delivery destination.
  Might seem obvious, but consider just a username
  e.g. "sam" — that would require an authority with
  knowledge of where "sam" wants their messages.
  Instead, simply sam@destination removed the need for
  a "directory authority."
- Is a "to" field required?
- Messages are encrypted at the ends
  - How painful would this be for group conversations?
    Kind of like the WhatsApp problem.
- Some smtp-like feature where it can be deliviered
  and handed over to another server and so on
  - Maybe using dns srv records
    - Probably can't implement it with a web app
      this way, since web browsers aren't capable
- Clients may choose to offer rich-text rendering by
  interpreting the body text as Markdown

## Message format

Messages are encoded as a series of sections with headers in plain text.
A section header ends with a newline character (U+000A.)
Some sections are required and some are optional.
Some sections have trailing data, arbitrary bytes with a size defined by the header.
(This is how a message's body and attachments are encoded.)

The message format is designed...
- To be easy to parse and build
- To be easy for a human to inspect (ie with a text editor)
- To be simple so that there's little room for mistakes and confusion
- To be extensible

### Required sections

    NAME     VALUE
    subject  <text>
    from     <address> [<name>]
    to       <address> [<name>]
    body     <bytesize>

### Optional sections

    NAME     VALUE                     NOTES
    time     <datetime> [<tzoffset>]   Defaults to UTC if tzoffset is not given
    file     <bytesize> <name>


### Example message

```
subject Hello hej
from    robin@address Robin Smith
time    2022-08-08 11:09:03 -0700
to      sam@address
body    5
Hello
file 11 hello.txt
Hello
world
file 16 evening time.txt
Goodbye
sunshine
```


### Message encoding specification

```abnf
message        = section*
section        = custom_section | std_section
custom_section = "x-" key (whitespace textline)? newline
std_section    = ( body_section | file_section
                 | to_section | from_section | subject_section | time_section
                 ) newline

body_section    = "body" whitespace bytesize newline anybyte{bytesize}
file_section    = "file" whitespace bytesize name newline anybyte{bytesize}
to_section      = "to" whitespace address name? newline
from_section    = "from" whitespace address name? newline
subject_section = "subject" whitespace textline newline
time_section    = "time" whitespace datetime [timezoneoffset] newline

address  = username "@" domain
username = (unicode_letter | unicode_digit | "_" | "-" | "+" | ".")+
domain   = (unicode_letter | unicode_digit | "_" | "-" | "+" | ".")+

datetime = year "-" month "-" day space hour ":" minute ":" second
year     = decdigit{4}
month    = decdigit{2}
day      = decdigit{2}
hour     = decdigit{2}
minute   = decdigit{2}
second   = decdigit{2}

timezoneoffset = "-"? tzhours
tzhours        = decdigit{4}

key        = (unicode_letter | unicode_digit | "_" | "-")+
name       = textline
textline   = <any Unicode character except 0+000A>
anybyte    = <byte 0x00–0xFF>
newline    = <byte 0x0A>
space      = <byte 0x20>
tab        = <byte 0x09>
whitespace = (tab | space)+
decdigit   = <byte 0x30–0x39>
```

