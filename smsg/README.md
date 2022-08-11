# smsg

CLI program implementing smolmsg

To build you'll need Go 1.18 or later.

    make

Run it with `-h` to see options:

    ./smsg -h

Messages are stored as files. Ie.

    ~/.smolmsg/
      /inbox/
        20220808-180903.msg
        20220808-120141.msg
      /outbox/
        20220808-191222.msg

The smsg program maintains an index at `~/.smolmsg/smsg.db`
which it builds from looking at the files in `~/.smolmsg/`.

There's an example directory to copy for development:

    cp example-smolmsg-dir ~/.smolmsg
    chmod -R 0700 ~/.smolmsg

