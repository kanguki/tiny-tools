package main

import (
	"bytes"
	"context"
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

	"golang.org/x/sync/errgroup"
)

const commandConvert = "ebook-convert"

func main() {
	input := flag.String("in", "", "input , either a file or a directory")
	maxHintLevel := flag.Int("hint", 5, "hint level, default is 5. 1 shows fewer wordwise hints, 5 shows all wordwise hints")
	parallel := flag.Int("parallel", runtime.NumCPU(), "number of files in a directory to process concurrently, default is number of cpu")
	maxDistance := flag.Int("max-distance", 1000, "max word distance to replace word again if it already appeared before. default is 1000")
	outFormat := flag.String("of", "epub", "output format of the wordwised book. Accept multiple formats in comma separated format. default is epub")
	outDir := flag.String("od", "", "directory to put output files to. if empty, put it the 'wordwise_generated' directory containing the book")
	flag.Parse()
	if *input == "" {
		log.Println("input is empty")
		log.Println("usage:", os.Args[0], "--in <file-or-dir> [--hint <1-5> --max-distance <1000> --parallel <8> --of <epub,pdf,azw3>]")
		os.Exit(1)
	}
	convertFormats := strings.Split(*outFormat, ",")
	log.Printf("Injecting wordwise for %s, hint %d, parallel %d, max-distance %d output %v\n", *input, *maxHintLevel, *parallel, *maxDistance, convertFormats)

	exe, err := newExecutor(*maxDistance, *parallel, *maxHintLevel, *input, *outDir, "", convertFormats)
	if err != nil {
		log.Fatal("newExecutor: ", err)
	}
	err = exe.generateWordwise()
	if err != nil {
		log.Fatal("newExecutor: ", err)
	}
}

type executor struct {
	wordwiseDict        map[string]WordwiseEntry
	maxDistance         int
	maxHintLevel        int
	outDirPath          string
	input               string
	generatedFolderName string
	outFormats          []string
	parallel            int
}

func newExecutor(maxDistance, maxHintLevel, parallel int, input, outDirPath, generatedFolderName string, outFormats []string) (*executor, error) {
	assertCalibreIsInstalled()
	wordwiseDict, err := loadWordwiseDict("wordwise-dict.csv")
	if err != nil {
		return nil, fmt.Errorf("loadWordwiseDict: %v", err)
	}
	if generatedFolderName == "" {
		generatedFolderName = "wordwise_generated"
	}
	if outDirPath == "" {
		// make a folder named generatedFolderName in the input dir
		inputInfo, iErr := os.Stat(input)
		if iErr != nil {
			return nil, fmt.Errorf("os.Stat(input): %v", iErr)
		}
		if !inputInfo.IsDir() {
			outDirPath = filepath.Join(filepath.Dir(input), generatedFolderName)
		} else {
			outDirPath = filepath.Join(input, generatedFolderName)
		}
	}
	err = os.MkdirAll(outDirPath, 0765)
	if err != nil {
		return nil, fmt.Errorf("os.MkdirAll(outDirPath): %v", err)
	}
	exe := executor{
		wordwiseDict:        wordwiseDict,
		maxDistance:         maxDistance,
		maxHintLevel:        maxHintLevel,
		outDirPath:          outDirPath,
		input:               input,
		generatedFolderName: generatedFolderName,
		outFormats:          outFormats,
		parallel:            parallel,
	}
	return &exe, nil
}

func (exe *executor) listAllChildFilesIfInputIsADir() ([]string, error) {
	inputInfo, iErr := os.Stat(exe.input)
	if iErr != nil {
		return nil, fmt.Errorf("os.Stat(input): %v", iErr)
	}
	if !inputInfo.IsDir() {
		return []string{exe.input}, nil
	}
	var files []string
	err := filepath.Walk(exe.input, func(path string, info os.FileInfo, err error) error {
		splitPaths := strings.Split(filepath.ToSlash(path), "/")
		for _, s := range splitPaths {
			if s == exe.generatedFolderName {
				// skip generated folder
				return nil
			}
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// input can be file or dir. if it's a dir, all files inside will be wordwised
func (exe *executor) generateWordwise() error {
	tasks := make(chan string, exe.parallel)
	inputs, err := exe.listAllChildFilesIfInputIsADir()
	if err != nil {
		return fmt.Errorf("exe.listFilesInDir: %w", err)
	}
	go func() {
		for _, input := range inputs {
			tasks <- input
		}
	}()

	eg, _ := errgroup.WithContext(context.Background())
	for i := 0; i < len(inputs); i++ {
		task := <-tasks
		eg.Go(func() error {
			return exe.generateWordwiseForAFile(task)
		})
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("eg.Wait: %w", err)
	}
	return nil
}

func (exe *executor) generateWordwiseForAFile(inFilePath string) error {
	outTempDir := nameTempDirHtml(strings.TrimSuffix(path.Base(inFilePath), path.Ext(inFilePath)))
	defer cleanOldTempFiles(outTempDir)
	log.Printf("[+] Convert Book to HTML, input %s", inFilePath)
	if err := convertToHTML(inFilePath, outTempDir); err != nil {
		log.Fatalf("Error converting to HTML: %v", err)
	}

	outTempHtml := path.Join(outTempDir, "index1.html")
	log.Println("[+] Load Book Contents")
	bookContent, err := os.ReadFile(outTempHtml)
	if err != nil {
		log.Fatalf("Error loading book content: %v", err)
	}
	start, stop := 0, len(bookContent)

	mediator := newReplacementMediator(exe.maxDistance)
	log.Println("[+] Start replacing")
	position := 0
	replaceWord := func(start int) (replacedWord string, nextStart int) {
		next := strings.Index(string(bookContent[start:]), " ")
		position++
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
		wordwise, isAWordWise := exe.wordwiseDict[keyword]
		if word == "" || // non-word string
			!isAWordWise ||
			wordwise.HintLevel > exe.maxHintLevel ||
			mediator.hasReplacedJustNow(keyword, position) {
			return originalWord, start
		}
		mediator.setLastReplacedPosition(keyword, position)
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
		log.Fatalf("Error creating new book content with Wordwised: %v", err)
	}

	log.Printf("[+] Converting html to %v\n", exe.outFormats)
	for _, format := range exe.outFormats {
		bookfilename := strings.TrimSuffix(filepath.Base(inFilePath), filepath.Ext(inFilePath))
		outputFilename := path.Join(exe.outDirPath, bookfilename+"."+format)
		var opts []string
		if err := convert(outTempHtml, outputFilename, opts...); err != nil {
			log.Printf("Error converting %s to %s: %v\n", outTempHtml, format, err)
		}
		log.Printf("output %s to \"%s\"\n", format, outputFilename)
	}

	log.Printf("[+] %d books %v with wordwise generation done!", len(exe.outFormats), exe.outFormats)
	return nil
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

func convertToHTML(input string, outputDir string) error {
	tempHtmlz := time.Now().Format("20060102150405") + "book_dump.htmlz"
	err := convert(input, tempHtmlz)
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
func convert(input, outputFile string, opts ...string) error {
	errBuf := bytes.NewBuffer(nil)
	cmdOpts := []string{input, outputFile}
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
