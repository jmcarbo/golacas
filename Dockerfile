FROM golang
ADD . /go/src/github.com/jmcarbo/golacas
RUN cd /go/src/github.com/jmcarbo/golacas && go install .
CMD /go/bin/golacas
