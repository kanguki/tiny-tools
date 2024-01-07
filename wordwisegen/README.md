# wordwisecreator

Clone https://github.com/xnohat/wordwisecreator in go because I can't make modifications to php code.

You need to install calibre on your device, such that command `convert-ebook` is available

```
usage: ./convert --in <file> [--hint <1-5> --max-distance <1000> --parallel <8> --of <epub,pdf,azw3>] --od <output/directory/default/to/wordwise/in/file/folder>
```

example

```
dir=some/dir
while read -r f ; do ./convert --in "$f" --hint 3 --of epub ; done < <(find $dir -type f \( -iname \*.mobi -o -iname \*.epub \) )
```

to customize your own word blob, modify **[wordwise-dict.csv](wordwise-dict.csv)**
