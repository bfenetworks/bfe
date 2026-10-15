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
    * 注：当前实现中 `period` 参数只支持空字符串，代表按日周期；传入非空字符串会报错

* 参数

| 参数       | 描述                    |
| ---------- | ----------------------- |
| start_time | String<br>起始时间      |
| end_time   | String<br>结束时间      |
| period     | String<br>周期, 当前只支持空字符串（代表日周期） |

时间格式：hhmmssZ，其中Z代表时区，详见附B说明

* 示例

```go
bfe_periodic_time_range("203000H", "204500H", "")
```

## time_weekday_in(weekday_list, zone)

* 语义: 判断当前时间所在的星期是否属于 weekday_list

* 参数

| 参数         | 描述                                                                             |
| ------------ | -------------------------------------------------------------------------------- |
| weekday_list | String<br>星期列表，多个之间使用 `\|` 连接，取值为 0-6；0=周日，1=周一，…，6=周六 |
| zone         | String<br>时区字母（可省略，详见附B）；省略时按 **Z（UTC）** 判定                 |

* 示例

```go
time_weekday_in("0|6", "H")                 // 周末（北京时间）
time_weekday_in("1|2|3|4|5", "H")           // 工作日（北京时间）
time_weekday_in("0|6", "H") && bfe_periodic_time_range("090000H", "180000H", "")
```

* 说明
    * 星期几取决于时区，`zone` 省略时按 UTC 判定；国内使用请显式写 `"H"`（UTC+8）。例如北京时间周一 00:00 时，UTC 仍是周日
    * `weekday_list` 允许重复（结果幂等），允许元素两侧空白；不支持区间写法（如 `"1-5"`，会报错）
    * 参数在配置加载时校验：非法星期、非法时区字母都会导致规则加载失败

## 附A.时间原语测试

- 为便于测试条件时间原语，可以在请求中增加 **X-Bfe-Debug-Time** 头部携带时间，来mock系统时间（`time_weekday_in` 同样受该头部影响）

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
