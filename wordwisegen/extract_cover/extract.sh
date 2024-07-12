# example

cat /tmp/covers | while read -r X ; do ebook-meta "$X" --get-cover "${X%.*}.jpg"  ; done
