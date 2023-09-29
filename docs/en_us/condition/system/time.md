# Time related primitives

## bfe_time_range(start_time, end_time)

* Description: Judge if current time is in [start_time, end_time]

* Parameters

| Parameter | Description |
| --------- | ---------- |
| start_time | String<br>start time |
| end_time | String<br> end time |

Time format：yyyymmddhhmmssZ，Z is time zone，detail information is shown in Section "Time Zone Detail"

* Example

```go
bfe_time_range("20190204203000H", "20190204204500H")
```

## bfe_periodic_time_range(start_time, end_time, period)

* Description: Judge if current time is periodly(period) in [start_time, end_time]

* Parameters

| Parameter | Description |
| --------- | ---------- |
| start_time | String<br>start time |
| end_time | String<br> end time |
| period | String<br> period, default *Day* |

Time format: hhmmssZ，Z is time zone，detail information is shown in "Appendix B: Time Zone Detail"

* Example

```go
bfe_periodic_time_range("203000H", "204500H", "")
```

## Appendix A: Condition Primitive Test

- In order to test time condition primitive,  **X-Bfe-Debug-Time** can be added in header of request to mock system time

## Appendix B: Time Zone Detail

| **Time zone name** | **Letter** | **Offset**                                             | **Description**     |
| :----------------- | :--------- | :----------------------------------------------------- | :------------------ |
| Alfa Time Zone     | A          | [+1](https://en.wikipedia.org/wiki/UTC%2B01:00)        |                     |
| Bravo Time Zone    | B          | [+2](https://en.wikipedia.org/wiki/UTC%2B02:00)        |                     |
| Charlie Time Zone  | C          | [+3](https://en.wikipedia.org/wiki/UTC%2B03:00)        |                     |
| Delta Time Zone    | D          | [+4](https://en.wikipedia.org/wiki/UTC%2B04:00)        |                     |
| Echo Time Zone     | E          | [+5](https://en.wikipedia.org/wiki/UTC%2B05:00)        |                     |
| Foxtrot Time Zone  | F          | [+6](https://en.wikipedia.org/wiki/UTC%2B06:00)        |                     |
| Golf Time Zone     | G          | [+7](https://en.wikipedia.org/wiki/UTC%2B07:00)        |                     |
| Hotel Time Zone    | **H**      | [+8](https://en.wikipedia.org/wiki/UTC%2B08:00)        | **Beijing Time**    |
| India Time Zone    | I          | [+9](https://en.wikipedia.org/wiki/UTC%2B09:00)        |                     |
| Kilo Time Zone     | K          | [+10](https://en.wikipedia.org/wiki/UTC%2B10:00)       |                     |
| Lima Time Zone     | L          | [+11](https://en.wikipedia.org/wiki/UTC%2B11:00)       |                     |
| Mike Time Zone     | M          | [+12](https://en.wikipedia.org/wiki/UTC%2B12:00)       |                     |
| November Time Zone | N          | [−1](https://en.wikipedia.org/wiki/UTC−01:00)          |                     |
| Oscar Time Zone    | O          | [−2](https://en.wikipedia.org/wiki/UTC−02:00)          |                     |
| Papa Time Zone     | P          | [−3](https://en.wikipedia.org/wiki/UTC−03:00)          |                     |
| Quebec Time Zone   | Q          | [−4](https://en.wikipedia.org/wiki/UTC−04:00)          |                     |
| Romeo Time Zone    | R          | [−5](https://en.wikipedia.org/wiki/UTC−05:00)          |                     |
| Sierra Time Zone   | S          | [−6](https://en.wikipedia.org/wiki/UTC−06:00)          |                     |
| Tango Time Zone    | T          | [−7](https://en.wikipedia.org/wiki/UTC−07:00)          |                     |
| Uniform Time Zone  | U          | [−8](https://en.wikipedia.org/wiki/UTC−08:00)          |                     |
| Victor Time Zone   | V          | [−9](https://en.wikipedia.org/wiki/UTC−09:00)          |                     |
| Whiskey Time Zone  | W          | [−10](https://en.wikipedia.org/wiki/UTC−10:00)         |                     |
| X-ray Time Zone    | X          | [−11](https://en.wikipedia.org/wiki/UTC−11:00)         |                     |
| Yankee Time Zone   | Y          | [−12](https://en.wikipedia.org/wiki/UTC−12:00)         |                     |
| Zulu Time Zone     | Z          | [0](https://en.wikipedia.org/wiki/Greenwich_Mean_Time) | Greenwich Mean Time |
