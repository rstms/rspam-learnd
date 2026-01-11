# rspam-learnd

HTTPS server for rspam message learning submission

Serve a TLS http endpoint requiring client certificates.  The POST /learn/
endpoint submits messages via rspamc.  All messages are stored in a queue
which allows the client to send multiple messages without waiting for
the rspamc results.
