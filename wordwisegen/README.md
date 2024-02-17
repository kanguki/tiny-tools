# wordwisecreator

Clone https://github.com/xnohat/wordwisecreator in go because I can't make modifications to php code.

May not be very useful for folks who doesn't know how to install stuff on local machines, i.e only for developers now ( I'm sorry 🥲 )

# requirements to build from source

- golang
- make
- calibre installed on your device, such that command `convert-ebook` is available

```
make
input="/path/to/some/file/or/folder"
./convert --in "$input" --hint 3 --of epub,pdf --od ./out
```

to customize your own word blob, modify **[wordwise-dict.csv](wordwise-dict.csv)** then run make again

# demo

origin
![1704626107057](image/README/1704626107057.png)

result![1704626025388](image/README/1704626025388.png)
