package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const commandConvert = "ebook-convert"

func main() {
	assertCalibreIsInstalled()
	inputFile := flag.String("in", "", "input file")
	maxHintLevel := flag.Int("hint", 5, "hint level, default is 5. 1 shows fewer wordwise hints, 5 shows all wordwise hints")
	maxDistance := flag.Int("max-distance", 1000*5, "max character distance to replace word again if it already appeared before. default is 5000, mo thinks it's ~ 1000 words")
	parallel := flag.Int("parallel", runtime.NumCPU(), "number of batches the file is split to process, default is number of cpu")
	outFormat := flag.String("of", "epub", "output format of the wordwised book. Accept multiple formats in comma separated format. default is epub")
	outDir := flag.String("od", "", "directory to put output files to. if empty, put it the 'wordwise' directory containing the book")
	flag.Parse()
	if *inputFile == "" {
		log.Println("input is empty")
		log.Println("usage:", os.Args[0], "--in <file> [--hint <1-5> --max-distance <1000> --parallel <8> --of <epub,pdf,azw3>]")
		os.Exit(1)
	}
	bookpath := filepath.Dir(*inputFile)
	bookfilename := strings.TrimSuffix(filepath.Base(*inputFile), filepath.Ext(*inputFile))
	convertFormats := strings.Split(*outFormat, ",")
	log.Printf("Injecting wordwise for %s, hint %d, parallel %d, output %v\n", *inputFile, *maxHintLevel, *parallel, convertFormats)

	stopwords, err := loadStopwords("stopwords.txt")
	if err != nil {
		log.Println("Error loading stopwords.txt:", err)
		os.Exit(1)
	}

	wordwiseDict, err := loadWordwiseDict("wordwise-dict.csv")
	if err != nil {
		log.Println("Error loading wordwise-dict.csv:", err)
		os.Exit(1)
	}

	outTempDir := nameTempDirHtml(strings.TrimSuffix(path.Base(*inputFile), path.Ext(*inputFile)))
	defer cleanOldTempFiles(outTempDir)

	log.Println("[+] Convert Book to HTML")
	if err := convertToHTML(*inputFile, outTempDir); err != nil {
		log.Printf("Error converting to HTML: %v", err)
		os.Exit(1)
	}

	outTempHtml := path.Join(outTempDir, "index1.html")
	log.Println("[+] Load Book Contents")
	bookContent, err := os.ReadFile(outTempHtml)
	if err != nil {
		log.Println("Error loading book content:", err)
		os.Exit(1)
	}
	start, stop := 0, len(bookContent)

	mediator := newReplacementMediator(*maxDistance)
	log.Println("[+] Start replacing")
	replaceWord := func(start int) (replacedWord string, nextStart int) {
		next := strings.Index(string(bookContent[start:]), " ")
		if next == -1 { // EOF
			return string(bookContent[start:]), -1
		}
		if next == 0 {
			return " ", start + 1
		}
		originalWord := string(bookContent[start : start+next])
		word := cleanWord(originalWord)
		start += next
		keyword := strings.ToLower(word)
		_, isStopword := stopwords[keyword]
		wordwise, isAWordWise := wordwiseDict[keyword]
		if word == "" || // non-word string
			isStopword ||
			!isAWordWise ||
			wordwise.HintLevel > *maxHintLevel ||
			mediator.hasReplacedJustNow(keyword, start) {
			return originalWord, start
		}
		mediator.setLastReplacedPosition(keyword, start)
		newWord := strings.ReplaceAll(originalWord, word, fmt.Sprintf("<ruby>%s<rt>%s</rt></ruby>", word, wordwise.ShortDef))
		return newWord, start
	}
	wordwisedContent := ""
	var newWord string
	for start < stop {
		newWord, start = replaceWord(start)
		if start == -1 {
			log.Println("[+] Done replacing")
			break
		}
		wordwisedContent += newWord
	}

	log.Println("[+] Done replacing. Writing replaced words back to temp html file")
	if err := os.WriteFile(outTempHtml, []byte(wordwisedContent), 0644); err != nil {
		log.Println("Error creating new book content with Wordwised:", err)
		os.Exit(1)
	}

	log.Printf("[+] Converting html to %v\n", convertFormats)
	for _, format := range convertFormats {
		outDir := *outDir
		if outDir == "" {
			outDir = path.Join(bookpath, "wordwise")
		}
		err = os.MkdirAll(outDir, 0765)
		if err != nil {
			log.Printf("creating out dir: %v\n", err)
			os.Exit(1)
		}
		outputFilename := path.Join(outDir, bookfilename+"."+format)
		var opts []string
		// if format == "epub" {
		// 	opts = append(opts, "--preserve-cover-aspect-ratio")
		// }
		if err := convert(outTempHtml, outputFilename, opts...); err != nil {
			log.Printf("Error converting %s to %s: %v\n", outTempHtml, format, err)
		}
		log.Printf("output %s to \"%s\"\n", format, outputFilename)
	}

	log.Printf("[+] %d books %v with wordwise generation done!", len(convertFormats), convertFormats)
}

// replacementMediator helps limits too many replacement for one word standing near each other
// for example, "Jack is an amateur. An amateur is blabla", then only first "amateur"
// is replaced
type replacementMediator struct {
	lastReplacedPosition map[string]int
	mut                  sync.RWMutex
	maxDistance          int
}

// maxDistance is the distance in which a word is considered far enough to be replaced again
func newReplacementMediator(maxDistance int) *replacementMediator {
	return &replacementMediator{
		lastReplacedPosition: make(map[string]int),
		mut:                  sync.RWMutex{},
		maxDistance:          maxDistance,
	}
}

func (r *replacementMediator) hasReplacedJustNow(word string, currentPosition int) bool {
	r.mut.RLock()
	lastReplacedPosition, ok := r.lastReplacedPosition[word]
	r.mut.RUnlock()
	yes := ok && lastReplacedPosition+r.maxDistance > currentPosition
	return yes
}

func (r *replacementMediator) setLastReplacedPosition(word string, postion int) {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.lastReplacedPosition[word] = postion
}

func nameTempDirHtml(input string) string {
	re_leadclose_whtsp := regexp.MustCompile(`^[\s\p{Zs}]+|[\s\p{Zs}]+$`)
	re_inside_whtsp := regexp.MustCompile(`[\s\p{Zs}]{2,}`)
	specialChars := regexp.MustCompile(`[^a-zA-Z0-9]`)
	final := re_leadclose_whtsp.ReplaceAllString(input, "")
	final = re_inside_whtsp.ReplaceAllString(final, " ")
	final = specialChars.ReplaceAllString(final, "-")
	return final
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func loadStopwords(filename string) (map[string]struct{}, error) {
	stopwords, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	ret := map[string]struct{}{}
	for _, word := range strings.Split(string(stopwords), "\n") {
		ret[word] = struct{}{}
	}
	return ret, nil
}

func loadWordwiseDict(filename string) (map[string]WordwiseEntry, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	csvReader := csv.NewReader(f)
	lines, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	headers := lines[0]
	//headers
	hdID, hdWord, hdFullDef, hdShortDef, hdExampleSentence, hdHintLevel := -1, -1, -1, -1, -1, -1
	_, _, _ = hdID, hdFullDef, hdExampleSentence
	for i, header := range headers {
		switch header {
		case "word":
			hdWord = i
		case "short_def":
			hdShortDef = i
		case "hint_level":
			hdHintLevel = i
		}
	}
	for i, hd := range []int{hdWord, hdShortDef, hdHintLevel} {
		if hd == -1 {
			return nil, fmt.Errorf("could not found header %d %v", i, headers)
		}
	}
	var wordwiseDict = map[string]WordwiseEntry{}

	for _, fields := range lines[1:] {
		wordwiseDict[strings.ToLower(fields[hdWord])] = WordwiseEntry{ShortDef: fields[hdShortDef], HintLevel: atoi(fields[hdHintLevel])}
	}

	return wordwiseDict, nil
}

func cleanOldTempFiles(path string) error {
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func assertCalibreIsInstalled() {
	_, err := exec.LookPath(commandConvert)
	if err != nil {
		log.Println("Please check if you have Calibre installed and can run the 'ebook-convert' command.")
		log.Println("This script requires Calibre to process ebook texts.")
		log.Fatal(err)
	}
}

func convertToHTML(inputFile string, outputDir string) error {
	tempHtmlz := time.Now().Format("20060102150405") + "book_dump.htmlz"
	err := convert(inputFile, tempHtmlz)
	if err != nil {
		return fmt.Errorf("convert to htmlz: %w", err)
	}
	err = convert(tempHtmlz, outputDir)
	if err != nil {
		return fmt.Errorf("convert htmlz to html: %w", err)
	}
	outputHtml := path.Join(outputDir, "index1.html")
	if _, err := os.Stat(outputHtml); os.IsNotExist(err) {
		log.Fatalf("expect output %s to be generated after converting htmlz to html, but it doesn't exist", outputHtml)
	}
	if err = os.RemoveAll(tempHtmlz); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", tempHtmlz, err)
	}
	return nil
}

// convert picks up file name and do the convert
// eg.g convert(xxx.epub, yyy.pdf)
func convert(inputFile, outputFile string, opts ...string) error {
	errBuf := bytes.NewBuffer(nil)
	cmdOpts := []string{inputFile, outputFile}
	if opts != nil {
		cmdOpts = append(cmdOpts, opts...)
	}
	cmd := exec.Command(commandConvert, cmdOpts...)
	cmd.Stderr = errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running command \n%s\n %w . details: %s", cmd.String(), err, errBuf.Bytes())
	}
	return nil
}

func cleanWord(word string) string {
	// Strip HTML tags
	res := strings.TrimSpace(word)

	// Strip special characters
	specialChars := ",<>;*&~/\"[]#?`–.'\"!“”:."
	for _, char := range specialChars {
		res = strings.ReplaceAll(res, string(char), "")
	}

	return res
}

type WordwiseEntry struct {
	ShortDef  string
	HintLevel int
}
