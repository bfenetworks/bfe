# 系统信号说明

## SIGQUIT

优雅退出BFE进程

!!! note
    BFE进程不再接收新的连接请求，继续完成活跃请求处理后退出, 或超过GracefulShutdownTimeout(conf/bfe.conf)后强制退出

## SIGTERM

强制退出BFE进程
