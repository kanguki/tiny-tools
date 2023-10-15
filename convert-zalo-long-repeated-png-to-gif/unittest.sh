#/bin/bash

./convert-zalo-long-repeated-png-to-gif.sh unittest.png
./convert-zalo-long-repeated-png-to-gif.sh unittest.png boy-send-heart1
./convert-zalo-long-repeated-png-to-gif.sh unittest.png boy-send-heart2 outgif

if [[ ! -f "gif/unittest.gif" ]] || [[ ! -f "gif/boy-send-heart1.gif" ]] || [[ ! -f "outgif/boy-send-heart2.gif" ]]; then
    echo "❌ oh no, missing expected files. plz check again "
    exit 1
else
    echo "✅ tests pass successfully"
fi