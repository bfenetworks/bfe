// Copyright (c) 2019 Baidu, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build darwin

package bfe_util

/*
#include <arpa/inet.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef unsigned long long uint64_t;
typedef unsigned int uint32_t;

static uint32_t Mod_Prime_List_1[256] = {
256,     65536,         3,       768,    196608,         9,      2304,    589824,
 27,      6912,   1769472,        81,     20736,   5308416,       243,     62208,
15925248,       729,    186624,  14221318,      2187,    559872,   9109528,      6561,
1679616,  10551371,     19683,   5038848,  14876900,     59049,  15116544,  11076274,
177147,  11795206,  16451609,    531441,   1831192,  15800401,   1594323,   5493576,
13846777,   4782969,  16480728,   7985905,  14348907,  15887758,   7180502,   9492295,
14108848,   4764293,  11699672,   8772118,  14292879,   1544590,   9539141,   9324211,
4633770,  11840210,  11195420,  13901310,   1966204,     31834,   8149504,   5898612,
95502,   7671299,    918623,    286506,   6236684,   2755869,    859518,   1932839,
8267607,   2578554,   5798517,   8025608,   7735662,    618338,   7299611,   6429773,
1855014,   5121620,   2512106,   5565042,  15364860,   7536318,  16695126,  12540154,
5831741,  16530952,   4066036,    718010,  16038430,  12198108,   2154030,  14560864,
3039898,   6462090,  10128166,   9119694,   2609057,  13607285,  10581869,   7827171,
7267429,  14968394,   6704300,   5025074,  11350756,   3335687,  15075222,    497842,
10007061,  11671240,   1493526,  13243970,   1459294,   4480578,   6177484,   4377882,
13441734,   1755239,  13133646,   6770776,   5265717,   5846512,   3535115,  15797151,
762323,  10605345,  13837027,   2286969,  15038822,   7956655,   6860907,  11562040,
7092752,   3805508,   1131694,   4501043,  11416524,   3395082,  13503129,    695146,
10185246,   6954961,   2085438,  13778525,   4087670,   6256314,   7781149,  12263010,
1991729,   6566234,   3234604,   5975187,   2921489,   9703812,   1148348,   8764467,
12334223,   3445044,   9516188,   3448243,  10335132,  11771351,  10344729,  14228183,
1759627,  14256974,   9130123,   5278881,   9216496,  10613156,  15836643,  10872275,
15062255,  13955503,  15839612,  11632339,   8312083,  13964410,   1342591,   8159036,
8338804,   4027773,   7699895,   8239199,  12083319,   6322472,   7940384,   2695531,
2190203,   7043939,   8086593,   6570609,   4354604,   7482566,   2934614,  13063812,
5670485,   8803842,   5637010,    234242,   9634313,    133817,    702726,  12125726,
401451,   2108178,   2822752,   1204353,   6324534,   8468256,   3613059,   2196389,
8627555,  10839177,   6589167,   9105452,  15740318,   2990288,  10539143,  13666528,
8970864,  14840216,   7445158,  10135379,  10966222,   5558261,  13628924,  16121453,
16674783,   7332346,  14809933,  16469923,   5219825,  10875373,  15855343,  15659475,
15848906,  14011603,  13423999,  13992292,   8480383,   6717571,   8422450,   8663936,
3375500,   8490137,   9214595,  10126500,   8693198,  10866572,  13602287,   9302381
};

static uint32_t Mod_Prime_List_2[256] = {
15824477,   7761153,   7145686,    580925,  14499208,   4036269,   9875725,  11605750,
1507777,    115335,  12748561,   8855010,   1960695,  15399149,  16317578,  16554616,
10127548,   8963642,  12993288,   4396326,   1387123,   2782309,   7628746,   6803892,
13744855,  12248289,  15002970,  15558948,   6894525,   3392505,  12844131,  16543731,
7340988,    246640,  12808243,   7356403,   4192880,  16413743,   7618458,   4170164,
10598447,  12073393,   3783992,  12401609,   3921293,  13996267,   9500965,  16330384,
3055753,  10521614,   9181344,   1616204,  11095448,   5088057,  10698269,   4073427,
2610974,  14098583,   2139463,  10832160,   4795125,   2816473,  16374730,  14408329,
14325643,   9935226,  10060807,   8655145,   1126852,   3261729,  12919873,   2379285,
5117796,   1534254,   6893447,   3116537,   9305119,  16525405,   2649532,   7192232,
12496701,  11487646,   4827551,  11117529,  10740793,  14959571,   4448804,  14821491,
2654722,   8520872,    307362,  11575876,  10637232,   5225154,  12240703,  13060954,
4941623,   6765563,   3932631,    121596,  14351377,  16523130,   2067132,   9092623,
12458026,   1586846,   3579800,  10460054,  10199183,  10525003,  10048928,   5614121,
11153061,   3059786,  11554062,   5052848,   1684765,  11869865,   2012421,  11863806,
461317,    656759,    358314,   7842389,  11164903,   6091338,  15880220,   5254162,
2889552,   1528556,   5434759,  15567986,   9208253,   8504908,  12997777,   5545510,
10365844,   2858622,  10387675,   8447358,  15042176,   8818485,   9387494,   4059007,
15696653,   8592607,   1894323,  15185116,  11856727,  15426292,   6488987,    237971,
10588979,   9649585,   4045507,  12240653,  13048154,   1664823,   6764713,   3715031,
11524792,  14336927,  12823930,  11372275,   8846973,  16680422,   8779486,  16180949,
15131990,  15033670,   6640949,   5585845,   3914405,  12232939,  11073370,  16213288,
6633575,   3698101,   7190712,  12107581,  12536120,   4801711,   4502489,  11787652,
14520291,   9433517,  15840895,  11964161,   9374998,    860031,   2064349,   8380175,
14620527,   1539535,   8245383,  13668173,   9394896,   5953919,  14255354,   8718441,
553429,   7460232,  13995905,   9408293,   9383551,   3049599,   8946190,   8525576,
1511586,   1090439,  10717200,   8919763,   1760264,  14420410,    641180,  13147289,
10266184,  10900060,   5400326,   6753138,    751831,   7919547,  14140152,  12781127,
414707,   5501798,  15952771,   7050019,   9644571,   2761923,   2409930,  12962916,
13398293,   7414412,   2265985,   9667394,   8604611,   4967347,  13350907,  12060795,
558904,   8861832,   3707127,   9501368,  16433552,  12689562,  10528465,  10935200,
14396166,  11211915,   1349211,   9854036,   6053366,   6159388,  16523821,   2244028
};

unsigned int getsign_24_1(char* str)
{
    int i = 0;
    unsigned int itemp = 0;

    while(str[i] != 0)
    {
        //% PRIME_USED_24
        int j = i + 1;
        itemp = ((unsigned char)(str[i]) * Mod_Prime_List_1[0xFF & (j)] + itemp);
    }

    return itemp;
}

unsigned int getsign_24_2(char* str)
{
    int i=0;
    unsigned int itemp = 0;

    while (str[i]!=0)
    {
        //% PRIME_USED_24;
        int j = i + 1;
        itemp = ((unsigned char)(str[i])*Mod_Prime_List_2[0xFF & (j)]+itemp);
    }

    return itemp;
}

void bfe_creat_sign_f64(char* psrc, int slen, uint32_t* sign1, uint32_t * sign2)
{
    *sign1=0;
    *sign2=0;

    if(slen <= 4)
    {
        memcpy(sign1, psrc, slen);
        return;
    }
    else
    {
        if(slen<=8)
        {
            memcpy(sign1, psrc, 4);
            memcpy(sign2, psrc+4, slen-4);
            return;
        }
        else
        {
            (*sign1) = getsign_24_1(psrc);
            (*sign2) = getsign_24_2(psrc);
            return;
        }
    }

    // should not comes to here
    return;
}

uint64_t calculate_log_id(char *network, char* client_addr, char* local_addr,
                          int pid, uint64_t sec, uint64_t usec)
{
    char connected_string[128] = {0};
    uint32_t connected_string_size = sizeof(connected_string);

	snprintf(connected_string, connected_string_size, "%s:%s/%s %lld %lld %d",
             network, client_addr, local_addr, sec, usec, pid);


    uint32_t high_int = 0;
    uint32_t low_int = 0;
    bfe_creat_sign_f64(connected_string, connected_string_size, &high_int, &low_int);

    uint64_t log_id = (uint64_t)(((uint64_t)(high_int) << 32) | (uint64_t)(low_int));

    return log_id;
}
*/
import "C" // this line must be separated

import (
	"net"
	"os"
	"time"
	"unsafe"
)

// calcLogId calculates a new log id.
//
// parameters:
//     network    - client Network, for exapmle, "tcp", "udp"
//     clientAddr - client Addr
//     localAddr  - local bfe server Addr
//     pid        - bfe process pid
//     sec        - seconds return from time.Unix()
//     usec       - micro seconds
// return:
//     an unsigned int64 id
func calcLogId(network string, clientAddr string, localAddr string,
	pid int, sec int64, usec int) uint64 {
	pNetwork := C.CString(network)
	pClientAddr := C.CString(clientAddr)
	pLocalAddr := C.CString(localAddr)
	id := C.calculate_log_id(pNetwork, pClientAddr, pLocalAddr, C.int(pid), C.ulonglong(sec), C.ulonglong(usec))
	C.free(unsafe.Pointer(pNetwork))
	C.free(unsafe.Pointer(pClientAddr))
	C.free(unsafe.Pointer(pLocalAddr))

	return uint64(id)
}

// GetLogID returns a new logid
func GetLogID(conn net.Conn) uint64 {
	pid := os.Getpid()
	t := time.Now()
	sec := t.Unix()
	usec := t.Nanosecond() / 1000

	network := conn.RemoteAddr().Network()
	clientAddr := conn.RemoteAddr().String()
	localAddr := conn.LocalAddr().String()

	logid := calcLogId(network, clientAddr, localAddr, pid, sec, usec)

	return logid
}
