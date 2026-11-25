CC ?= gcc
CFLAGS ?= -O2 -Wall -Wextra -s

all: sysopt

sysopt: sysopt.c
	$(CC) $(CFLAGS) -o sysopt sysopt.c

clean:
	rm -f sysopt
