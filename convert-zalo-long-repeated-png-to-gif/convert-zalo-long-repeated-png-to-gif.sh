#!/bin/bash
set -eo pipefail

if [[ $# -eq 0 ]]; then
    echo "usage: $(basename $0) <input-file.png> [optional-output-name-of-gif-default-is-name-of-input-file] [optional-output-dir-default-is-dir-gif-in-crrent-dir]"
    echo "example: $(basename $0) a-long-image.png [a-beautiful-day] [out]"
    exit 1
fi

output_name=$(basename "$1")
output_name=${output_name%.*} #remove extenstion
output_name=${output_name// /} #remove whitespaces
if [[ $# -ge 2 ]]; then
    output_name="$2"
fi

output_dir="gif"
if [[ $# -ge 3 ]]; then
    output_dir="$3"
fi

if [ ! -d "$output_dir" ]; then
    mkdir -p "$output_dir"
fi

img_width=$(( $(identify -ping -format '%w' "$1") ))
img_height=$(( $(identify -ping -format '%h' "$1") ))
repeated_images=$(( $img_width / $img_height ))

FIXED_HEIGHT_OF_A_ZALO_GIF_PNG=130

if [[ $img_height -eq $FIXED_HEIGHT_OF_A_ZALO_GIF_PNG ]] && [[ $repeated_images -gt 1 ]]  ; then
    echo "$1" is a gif file: width $img_width , height $img_height, repeated_images $repeated_images
    concat_images_to_sequence_to_use_in_convert=""
    # form a sequence of 
    for ((x=0; x<repeated_images; x++)); do
        if [ -n "$concat_images_to_sequence_to_use_in_convert" ]; then
            concat_images_to_sequence_to_use_in_convert="${concat_images_to_sequence_to_use_in_convert} "
        fi
        concat_images_to_sequence_to_use_in_convert="${concat_images_to_sequence_to_use_in_convert} ${output_name}-$x.png"
    done
    # split to multiple images
    convert "$1" -crop ${repeated_images}x1@ "${output_name}-%d.png"
    # convert multiple images to gif
    convert -dispose previous  -delay 10 -loop 0 +repage $concat_images_to_sequence_to_use_in_convert "${output_dir}/${output_name}.gif"
    echo converted to "${output_dir}/${output_name}.gif" 
    # clean up split files
    rm "${output_name}"-*.png
fi

