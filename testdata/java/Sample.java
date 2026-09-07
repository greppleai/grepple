package testdata.java;

import java.util.List;

public class Sample {
    record User(String id, String name) {}

    public static String formatUser(User user) {
        return user.name().trim() + " (" + user.id() + ")";
    }

    public static void printUsers(List<User> users) {
        for (User user : users) {
            System.out.println(formatUser(user));
        }
    }
}
