# 时间相关条件原语

## bfe_time_range(start_time, end_time)

* 语义: 判断当前时间是否属于[start_time, end_time]

* 参数

| 参数       | 描述                   |
| ---------- | ---------------------- |
| start_time | String<br>起始时间     |
| end_time   | String<br>结束时间     |

时间格式：yyyymmddhhmmssZ，其中Z代表时区，详见附B说明

* 示例

```go
bfe_time_range("20190204203000H", "20190204204500H")
```

## bfe_periodic_time_range(start_time, end_time, period)

* 语义: 判断当前时间是否周期性属于[start_time, end_time]

* 参数

| 参数       | 描述                    |
| ---------- | ----------------------- |
| start_time | String<br>起始时间      |
| end_time   | String<br>结束时间      |
| period     | String<br>周期, 缺省代表日 |

时间格式：hhmmssZ，其中Z代表时区，详见附B说明

* 示例

```go
bfe_periodic_time_range("203000H", "204500H", "")
```

## 附A.时间原语测试

- 为便于测试条件时间原语，可以在请求中增加 **X-Bfe-Debug-Time** 头部携带时间，来mock系统时间

## 附B.时区字符编码

| **Time zone name** | **Letter** | **Offset**                                             | **说明**         |
| :----------------- | :--------- | :----------------------------------------------------- | :--------------- |
| Alfa Time Zone     | A          | [+1](https://en.wikipedia.org/wiki/UTC%2B01:00)        |                  |
| Bravo Time Zone    | B          | [+2](https://en.wikipedia.org/wiki/UTC%2B02:00)        |                  |
| Charlie Time Zone  | C          | [+3](https://en.wikipedia.org/wiki/UTC%2B03:00)        |                  |
| Delta Time Zone    | D          | [+4](https://en.wikipedia.org/wiki/UTC%2B04:00)        |                  |
| Echo Time Zone     | E          | [+5](https://en.wikipedia.org/wiki/UTC%2B05:00)        |                  |
| Foxtrot Time Zone  | F          | [+6](https://en.wikipedia.org/wiki/UTC%2B06:00)        |                  |
| Golf Time Zone     | G          | [+7](https://en.wikipedia.org/wiki/UTC%2B07:00)        |                  |
| Hotel Time Zone    | **H**      | [+8](https://en.wikipedia.org/wiki/UTC%2B08:00)        | **北京标准时间** |
| India Time Zone    | I          | [+9](https://en.wikipedia.org/wiki/UTC%2B09:00)        |                  |
| Kilo Time Zone     | K          | [+10](https://en.wikipedia.org/wiki/UTC%2B10:00)       |                  |
| Lima Time Zone     | L          | [+11](https://en.wikipedia.org/wiki/UTC%2B11:00)       |                  |
| Mike Time Zone     | M          | [+12](https://en.wikipedia.org/wiki/UTC%2B12:00)       |                  |
| November Time Zone | N          | [−1](https://en.wikipedia.org/wiki/UTC−01:00)          |                  |
| Oscar Time Zone    | O          | [−2](https://en.wikipedia.org/wiki/UTC−02:00)          |                  |
| Papa Time Zone     | P          | [−3](https://en.wikipedia.org/wiki/UTC−03:00)          |                  |
| Quebec Time Zone   | Q          | [−4](https://en.wikipedia.org/wiki/UTC−04:00)          |                  |
| Romeo Time Zone    | R          | [−5](https://en.wikipedia.org/wiki/UTC−05:00)          |                  |
| Sierra Time Zone   | S          | [−6](https://en.wikipedia.org/wiki/UTC−06:00)          |                  |
| Tango Time Zone    | T          | [−7](https://en.wikipedia.org/wiki/UTC−07:00)          |                  |
| Uniform Time Zone  | U          | [−8](https://en.wikipedia.org/wiki/UTC−08:00)          |                  |
| Victor Time Zone   | V          | [−9](https://en.wikipedia.org/wiki/UTC−09:00)          |                  |
| Whiskey Time Zone  | W          | [−10](https://en.wikipedia.org/wiki/UTC−10:00)         |                  |
| X-ray Time Zone    | X          | [−11](https://en.wikipedia.org/wiki/UTC−11:00)         |                  |
| Yankee Time Zone   | Y          | [−12](https://en.wikipedia.org/wiki/UTC−12:00)         |                  |
| Zulu Time Zone     | Z          | [0](https://en.wikipedia.org/wiki/Greenwich_Mean_Time) | 格林威治标准时间 |
